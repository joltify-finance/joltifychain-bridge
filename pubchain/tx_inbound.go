package pubchain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"math/big"
	"strings"

	"github.com/cenkalti/backoff"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	zlog "github.com/rs/zerolog/log"
	bcommon "gitlab.com/oppy-finance/oppy-bridge/common"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/types"

	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/generated"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
)

const alreadyKnown = "already known"

func (pi *Instance) retrieveAddrfromRawTx(tx *ethTypes.Transaction) (types.AccAddress, error) { //nolint
	v, r, s := tx.RawSignatureValues()
	signer := ethTypes.LatestSignerForChainID(tx.ChainId())
	plainV := misc.RecoverRecID(tx.ChainId().Uint64(), v)
	sigBytes := misc.MakeSignature(r, s, plainV)

	sigPublicKey, err := crypto.Ecrecover(signer.Hash(tx).Bytes(), sigBytes)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to recover the public key")
		return types.AccAddress{}, err
	}

	transferFrom, err := misc.EthSignPubKeyToOppyAddr(sigPublicKey)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to recover the oppy Address")
		return types.AccAddress{}, err
	}
	return transferFrom, nil
}

func (pi *Instance) getBalance(value *big.Int) (types.Coin, error) {
	total := types.NewCoin(config.OutBoundDenomFee, types.NewIntFromBigInt(value))
	if total.IsNegative() {
		pi.logger.Error().Msg("incorrect amount")
		return types.Coin{}, errors.New("insufficient fund")
	}
	return total, nil
}

// ProcessInBoundERC20 process the inbound contract token top-up
func (pi *Instance) ProcessInBoundERC20(tx *ethTypes.Transaction, txInfo *Erc20TxInfo, txBlockHeight uint64) error {
	err := pi.processInboundERC20Tx(tx.Hash().Hex()[2:], txBlockHeight, txInfo.fromAddr, txInfo.tokenAddress, txInfo.Amount, txInfo.tokenAddress)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to process the inbound tx")
		return err
	}
	return nil
}

// ProcessNewBlock process the blocks received from the public pub_chain
func (pi *Instance) ProcessNewBlock(number *big.Int) error {
	block, err := pi.GetBlockByNumberWithLock(number)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to retrieve the block")
		return err
	}
	// we need to put the block height in which we find the tx
	pi.processEachBlock(block, number.Int64())
	return nil
}

func (pi *Instance) processInboundERC20Tx(txID string, txBlockHeight uint64, from types.AccAddress, to common.Address, value *big.Int, addr common.Address) error {
	// this is repeated check for tokenAddr which is cheked at function 'processEachBlock'
	tokenItem, exit := pi.TokenList.GetTokenInfoByAddress(strings.ToLower(addr.Hex()))
	if !exit {
		pi.logger.Error().Msgf("Token is not on our token list")
		return errors.New("token is not on our token list")
	}

	token := types.Coin{
		Denom:  tokenItem.Denom,
		Amount: types.NewIntFromBigInt(value),
	}

	tx := InboundTx{
		txID,
		from,
		txBlockHeight,
		token,
	}

	txIDBytes, err := hex.DecodeString(txID)
	if err != nil {
		pi.logger.Warn().Msgf("invalid tx ID %v\n", txIDBytes)
		return nil
	}

	delta := types.Precision - tokenItem.Decimals
	if delta != 0 {
		adjustedTokenAmount := bcommon.AdjustInt(tx.Token.Amount, int64(delta))
		tx.Token.Amount = adjustedTokenAmount
	}

	item := bcommon.NewAccountInboundReq(tx.Address, to, tx.Token, txIDBytes, int64(txBlockHeight))
	pi.AddItem(&item)
	return nil
}

func (pi *Instance) checkErc20(data []byte, to string) (*Erc20TxInfo, error) {
	// address toAddress, uint256 amount, address contractAddress, bytes memo

	// check it is from our smart contract
	if !strings.EqualFold(to, OppyContractAddress) {
		return nil, errors.New("not our smart contract")
	}

	if method, ok := pi.tokenAbi.Methods["oppyTransfer"]; ok {
		if len(data) < 4 {
			return nil, errors.New("invalid data")
		}
		params, err := method.Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		if len(params) != 4 {
			return nil, errors.New("invalid transfer parameter")
		}
		toAddr, ok := params[0].(common.Address)
		if !ok {
			return nil, errors.New("not valid address")
		}
		amount, ok := params[1].(*big.Int)
		if !ok {
			return nil, errors.New("not valid amount")
		}
		tokenAddress, ok := params[2].(common.Address)
		if !ok {
			return nil, errors.New("not valid address")
		}
		memo, ok := params[3].([]byte)
		if !ok {
			return nil, errors.New("not valid memo")
		}
		var memoInfo bcommon.BridgeMemo
		err = json.Unmarshal(memo, &memoInfo)
		if err != nil {
			return nil, err
		}

		fromAddr, err := types.AccAddressFromBech32(memoInfo.Dest)
		if err != nil {
			return nil, err
		}
		ret := Erc20TxInfo{
			fromAddr:     fromAddr,
			toAddr:       toAddr,
			Amount:       amount,
			tokenAddress: tokenAddress,
		}

		return &ret, nil
	}
	return nil, errors.New("invalid method for decode")
}

func (pi *Instance) processEachBlock(block *ethTypes.Block, txBlockHeight int64) {
	for _, tx := range block.Transactions() {
		if tx.To() == nil {
			continue
		}
		status, err := pi.checkEachTx(tx.Hash())
		if err != nil || status != 1 {
			continue
		}

		txInfo, err := pi.checkErc20(tx.Data(), tx.To().Hex())
		if err == nil {
			_, exit := pi.TokenList.GetTokenInfoByAddress(txInfo.tokenAddress.String())
			if !exit {
				// this indicates it is not to our smart contract
				continue
			}
			// process the public chain inbound message to the channel
			if !pi.checkToBridge(txInfo.toAddr) {
				pi.logger.Warn().Msg("the top up message is not to the bridge, ignored")
				continue
			}

			err := pi.ProcessInBoundERC20(tx, txInfo, block.NumberU64())
			if err != nil {
				zlog.Logger.Error().Err(err).Msg("fail to process the inbound contract message")
				continue
			}
			continue
		}
		if pi.checkToBridge(*tx.To()) {
			var memoInfo bcommon.BridgeMemo
			err = json.Unmarshal(tx.Data(), &memoInfo)
			if err != nil {
				pi.logger.Error().Err(err).Msgf("fail to unmarshal the memo")
				continue
			}

			fromAddr, err := types.AccAddressFromBech32(memoInfo.Dest)
			if err != nil {
				pi.logger.Error().Err(err).Msgf("fail to the acc address")
				continue
			}

			// this indicates it is a native bnb transfer
			balance, err := pi.getBalance(tx.Value())
			if err != nil {
				continue
			}
			tokenItem, exist := pi.TokenList.GetTokenInfoByAddress("native")
			if !exist {
				panic("native token is not set")
			}
			delta := types.Precision - tokenItem.Decimals
			if delta != 0 {
				adjustedTokenAmount := bcommon.AdjustInt(balance.Amount, int64(delta))
				balance.Amount = adjustedTokenAmount
			}

			item := bcommon.NewAccountInboundReq(fromAddr, *tx.To(), balance, tx.Hash().Bytes(), txBlockHeight)
			// we add to the retry pool to  sort the tx
			pi.AddItem(&item)
		}
	}
}

// UpdatePool update the tss pool address
func (pi *Instance) UpdatePool(pool *vaulttypes.PoolInfo) error {
	if pool == nil {
		return errors.New("nil pool")
	}
	poolPubKey := pool.CreatePool.PoolPubKey
	addr, err := misc.PoolPubKeyToOppyAddress(poolPubKey)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to convert the oppy address to eth address %v", poolPubKey)
		return err
	}

	ethAddr, err := misc.PoolPubKeyToEthAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the oppy address to eth address %v", poolPubKey)
		return err
	}

	pi.poolLocker.Lock()
	defer pi.poolLocker.Unlock()

	p := bcommon.PoolInfo{
		Pk:          poolPubKey,
		OppyAddress: addr,
		EthAddress:  ethAddr,
		PoolInfo:    pool,
	}

	if pi.lastTwoPools[1] != nil {
		pi.lastTwoPools[0] = pi.lastTwoPools[1]
	}
	pi.lastTwoPools[1] = &p
	return nil
}

// GetPool get the latest two pool address
func (pi *Instance) GetPool() []*bcommon.PoolInfo {
	pi.poolLocker.RLock()
	defer pi.poolLocker.RUnlock()
	var ret []*bcommon.PoolInfo
	ret = append(ret, pi.lastTwoPools...)
	return ret
}

// GetPool get the latest two pool address
func (pi *Instance) checkToBridge(dest common.Address) bool {
	pools := pi.GetPool()
	for _, el := range pools {
		if el != nil && dest.String() == el.EthAddress.String() {
			return true
		}
	}
	return false
}

func (pi *Instance) AddMoveFundItem(pool *bcommon.PoolInfo, height int64) {
	pi.moveFundReq.Store(height, pool)
}

func (pi *Instance) ExportMoveFundItems() []*bcommon.PoolInfo {
	var data []*bcommon.PoolInfo
	pi.moveFundReq.Range(func(key, value any) bool {
		exported := value.(*bcommon.PoolInfo)
		exported.Height = key.(int64)
		data = append(data, exported)
		return true
	})
	return data
}

// PopMoveFundItemAfterBlock pop up the item after the given block duration
func (pi *Instance) PopMoveFundItemAfterBlock(currentBlockHeight int64) (*bcommon.PoolInfo, int64) {
	min := int64(math.MaxInt64)
	pi.moveFundReq.Range(func(key, value interface{}) bool {
		h := key.(int64)
		if h <= min {
			min = h
		}
		return true
	})
	if min < math.MaxInt64 && (currentBlockHeight-min > config.MINCHECKBLOCKGAP) {
		item, _ := pi.moveFundReq.LoadAndDelete(min)
		return item.(*bcommon.PoolInfo), min
	}
	return nil, 0
}

// func (pi *Instance) moveBnb(senderPk string, receiver common.Address, amount *big.Int, nonce uint64, blockHeight int64) (string, error) {
//	ctx, cancel := context.WithTimeout(context.Background(), config.QueryTimeOut)
//	defer cancel()
//	chainID, err := pi.EthClient.NetworkID(ctx)
//	if err != nil {
//		pi.logger.Error().Err(err).Msg("fail to get the chain ID")
//		return "", err
//	}
//
//	gasPrice, err := pi.EthClient.SuggestGasPrice(context.Background())
//	if err != nil {
//		return "", err
//	}
//
//	gasLimit, err := pi.EthClient.EstimateGas(context.Background(), ethereum.CallMsg{
//		To:   &receiver,
//		Data: nil,
//	})
//	if err != nil {
//		return "", err
//	}
//
//	totalBnb := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(gasLimit))
//
//	totalBnbDec := types.NewDecFromBigIntWithPrec(totalBnb, types.Precision)
//	totalBnbDec = totalBnbDec.Mul(types.MustNewDecFromStr(config.GASFEERATIO))
//
//	moveFund := amount.Sub(amount, totalBnbDec.BigInt())
//	moveFundS := types.NewDecFromBigIntWithPrec(moveFund, types.Precision)
//	if moveFund.Cmp(big.NewInt(0)) != 1 {
//		pi.logger.Warn().Msgf("we do not have any bnb to move")
//		return "", nil
//	}
//
//	pi.logger.Info().Msgf("we need to move %v bnb", moveFundS.String())
//
//	dustBnb, err := types.NewDecFromStr(config.DUSTBNB)
//	if err != nil {
//		panic("invalid parameter")
//	}
//
//	if moveFund.Cmp(dustBnb.BigInt()) != 1 {
//		return "", nil
//	}
//	baseTx := ethTypes.LegacyTx{
//		Nonce:    nonce,
//		GasPrice: gasPrice,
//		Gas:      gasLimit,
//		To:       &receiver,
//		Data:     nil,
//		Value:    moveFund,
//	}
//
//	rawTx := ethTypes.NewTx(&baseTx)
//	signer := ethTypes.LatestSignerForChainID(chainID)
//	msg := signer.Hash(rawTx).Bytes()
//	signature, err := pi.tssSign(msg, senderPk, blockHeight)
//	if err != nil || len(signature) != 65 {
//		return "", errors.New("fail to get the valid signature")
//	}
//	bTx, err := rawTx.WithSignature(signer, signature)
//	if err != nil {
//		return "", err
//	}
//
//	err = pi.EthClient.SendTransaction(ctx, bTx)
//	if err != nil {
//		if err.Error() == "already known" {
//			pi.logger.Warn().Msgf("the tx has been submitted by others")
//			return rawTx.Hash().Hex(), nil
//		} else {
//			return "", err
//		}
//	}
//
//	return rawTx.Hash().Hex(), nil
//}

func (pi *Instance) moveERC20Token(index int, sender, receiver common.Address, balance *big.Int, tokenAddr string, tssReqChan chan *TssReq, tssRespChan chan map[string][]byte) (common.Hash, error) {
	txHash, err := pi.SendTokenBatch(index, sender, receiver, balance, nil, tokenAddr, tssReqChan, tssRespChan)
	if err != nil {
		if err.Error() == alreadyKnown {
			pi.logger.Warn().Msgf("the tx has been submitted by others")
			return txHash, nil
		}
		pi.logger.Error().Err(err).Msgf("fail to send the token with err %v for amount %v ", err, balance)
		return txHash, err
	}
	return txHash, nil
}

func (pi *Instance) doMoveTokenFunds(index int, previousPool *bcommon.PoolInfo, receiver common.Address, tokenAddr string, ethClient *ethclient.Client, tssReqChan chan *TssReq, tssRespChan chan map[string][]byte) (bool, error) {
	tokenInstance, err := generated.NewToken(common.HexToAddress(tokenAddr), ethClient)
	if err != nil {
		return false, err
	}
	balance, err := tokenInstance.BalanceOf(&bind.CallOpts{}, previousPool.EthAddress)
	if err != nil {
		return false, err
	}

	tick := html.UnescapeString("&#" + "9193" + ";")
	pi.logger.Info().Msgf(" %v we move fund %v %v from %v to %v", tick, tokenAddr, balance, previousPool.EthAddress.String(), receiver.String())

	if balance.Cmp(big.NewInt(0)) == 0 {
		return true, nil
	}

	if balance.Cmp(big.NewInt(0)) == 1 {
		erc20TxHash, err := pi.moveERC20Token(index, previousPool.EthAddress, receiver, balance, tokenAddr, tssReqChan, tssRespChan)
		// if we fail erc20 token transfer, we should not transfer the bnb otherwise,we do not have enough fee to pay retry
		if err != nil {
			return false, errors.New("fail to transfer erc20 token")
		}

		err1 := pi.CheckTxStatus(erc20TxHash.Hex())
		if err1 != nil {
			return false, err1
		}

		nowBalance, err2 := tokenInstance.BalanceOf(&bind.CallOpts{}, previousPool.EthAddress)
		if err2 == nil && nowBalance.Cmp(big.NewInt(0)) == 0 {
			tick = html.UnescapeString("&#" + "127974" + ";")
			zlog.Logger.Info().Msgf(" %v we have moved the erc20 %v with hash %v", tick, balance.String(), erc20TxHash)
			return true, nil
		}
		return false, nil
	}
	pi.logger.Warn().Msg("0 ERC20 balance do not need to move")

	return false, errors.New("we failed to move fund for this token")
}

func (pi *Instance) doMoveBNBFunds(previousPool *bcommon.PoolInfo, receiver common.Address) (bool, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.QueryTimeOut)
	defer cancel()
	balanceBnB, err := pi.getBalanceWithLock(ctx, previousPool.EthAddress)
	if err != nil {
		return false, false, err
	}

	tick := html.UnescapeString("&#" + "9193" + ";")
	pi.logger.Info().Msgf(" %v we move fund bnb:%v from %v to %v", tick, balanceBnB, previousPool.EthAddress.String(), receiver.String())

	// we move the bnb
	nonce, err := pi.getPendingNonceWithLock(ctx, previousPool.EthAddress)
	if err != nil {
		return false, false, err
	}

	bnbTxHash, emptyAccount, err := pi.SendNativeTokenForMoveFund(previousPool.Pk, previousPool.EthAddress, receiver, balanceBnB, new(big.Int).SetUint64(nonce))
	// bnbTxHash, err = pi.moveBnb(previousPool.Pk, receiver, balanceBnB, nonce, blockHeight)
	if err != nil {
		return false, false, err
	}
	if emptyAccount {
		zlog.Logger.Info().Msgf("this is the empty account to move fund")
		return true, true, nil
	}

	errCheck := pi.CheckTxStatus(bnbTxHash.Hex())
	if errCheck != nil {
		return false, false, errCheck
	}
	tick = html.UnescapeString("&#" + "127974" + ";")
	zlog.Logger.Info().Msgf(" %v we have moved the fund in the publicchain (BNB): %v with hash %v", tick, balanceBnB.String(), bnbTxHash)

	nowBalanceBnB, err := pi.getBalanceWithLock(ctx, previousPool.EthAddress)
	if err != nil {
		return false, false, nil
	}

	_, price, _, gas, err := pi.GetFeeLimitWithLock()
	if err != nil {
		return false, false, err
	}

	adjGas := int64(float32(gas) * config.MoveFundPubChainGASFEERATIO)
	fee := new(big.Int).Mul(price, big.NewInt(adjGas))

	// this statement is useful in
	if nowBalanceBnB.Cmp(fee) != 1 {
		return true, true, nil
	}

	return true, false, nil
}

func (pi *Instance) checkEachTx(h common.Hash) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.QueryTimeOut)
	defer cancel()
	receipt, err := pi.getTransactionReceiptWithLock(ctx, h)
	if err != nil {
		return 0, err
	}
	return receipt.Status, nil
}

// CheckTxStatus check whether the tx is already in the chain
func (pi *Instance) CheckTxStatus(hashStr string) error {
	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(submitBackoff), 30)

	var status uint64
	op := func() error {
		txHash := common.HexToHash(hashStr)
		ret, err := pi.checkEachTx(txHash)
		if err != nil {
			return err
		}
		status = ret
		return nil
	}

	err := backoff.Retry(op, bf)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to find the tx %v", hashStr)
		return err
	}
	if status != 1 {
		pi.logger.Warn().Msgf("the tx is failed, we need to redo the tx")
		return errors.New("tx failed")
	}
	pi.logger.Info().Msgf("we have successfully check the tx.")
	return nil
}
