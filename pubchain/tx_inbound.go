package pubchain

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/cenkalti/backoff"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	zlog "github.com/rs/zerolog/log"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
	"html"
	"math"
	"math/big"

	"github.com/cosmos/cosmos-sdk/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

// ProcessInBoundERC20 process the inbound contract token top-up
func (pi *PubChainInstance) ProcessInBoundERC20(tx *ethTypes.Transaction, tokenAddr, transferTo common.Address, amount *big.Int, blockHeight uint64) error {
	v, r, s := tx.RawSignatureValues()
	signer := ethTypes.LatestSignerForChainID(tx.ChainId())
	plainV := misc.RecoverRecID(tx.ChainId().Uint64(), v)
	sigBytes := misc.MakeSignature(r, s, plainV)

	sigPublicKey, err := crypto.Ecrecover(signer.Hash(tx).Bytes(), sigBytes)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to recover the public key")
		return err
	}

	transferFrom, err := misc.EthSignPubKeyToJoltAddr(sigPublicKey)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to recover the joltify Address")
		return err
	}

	err = pi.processInboundTx(tx.Hash().Hex()[2:], blockHeight, transferFrom, transferTo, amount, tokenAddr)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to process the inbound tx")
		return err
	}
	return nil
}

// ProcessNewBlock process the blocks received from the public pub_chain
func (pi *PubChainInstance) ProcessNewBlock(number *big.Int, joltifyBlockHeight int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	block, err := pi.EthClient.BlockByNumber(ctx, number)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to retrieve the block")
		return err
	}
	pi.processEachBlock(block, joltifyBlockHeight)
	return nil
}

// updateInboundTx update the top-up token with fee
func (pi *PubChainInstance) updateInboundTx(txID string, amount *big.Int, blockNum uint64) *inboundTx {
	data, ok := pi.pendingInbounds.Load(txID)
	if !ok {
		pi.logger.Warn().Msgf("inbound fail to get the stored tx from pool with %v\n", pi.pendingInbounds)
		inBnB := inboundTxBnb{
			blockHeight: blockNum,
			txID:        txID,
			fee:         sdk.NewCoin(config.InBoundDenomFee, sdk.NewIntFromBigInt(amount)),
		}
		pi.pendingInboundsBnB.Store(txID, &inBnB)
		return nil
	}

	thisAccount := data.(*inboundTx)
	thisAccount.fee.Amount = thisAccount.fee.Amount.Add(types.NewIntFromBigInt(amount))
	err := thisAccount.Verify()
	if err != nil {
		pi.pendingInbounds.Store(txID, thisAccount)
		pi.logger.Warn().Msgf("the account cannot be processed on joltify pub_chain this round with err %v\n", err)
		return nil
	}
	// since this tx is processed,we do not need to store it any longer
	pi.pendingInbounds.Delete(txID)
	return thisAccount
}

func (pi *PubChainInstance) processInboundTx(txID string, blockHeight uint64, from types.AccAddress, to common.Address, value *big.Int, addr common.Address) error {
	_, ok := pi.pendingInbounds.Load(txID)
	if ok {
		pi.logger.Error().Msgf("the tx already exist!!")
		return errors.New("tx existed")
	}

	if addr.String() != pi.tokenAddr {
		pi.logger.Error().Msgf("incorrect top up token")
		return errors.New("incorrect top up token")
	}

	token := types.Coin{
		Denom:  config.InBoundDenom,
		Amount: types.NewIntFromBigInt(value),
	}

	inTxBnB, ok := pi.pendingInboundsBnB.LoadAndDelete(txID)
	if !ok {
		fee := types.Coin{
			Denom:  config.InBoundDenomFee,
			Amount: types.NewInt(0),
		}

		tx := inboundTx{
			from,
			blockHeight,
			token,
			fee,
		}
		pi.logger.Info().Msgf("we add the tokens tx(%v):%v", txID, tx.token.String())
		pi.pendingInbounds.Store(txID, &tx)
		return nil
	}
	fee := inTxBnB.(*inboundTxBnb).fee
	tx := inboundTx{
		from,
		blockHeight,
		token,
		fee,
	}
	err := tx.Verify()
	if err != nil {
		pi.pendingInbounds.Store(txID, &tx)
		pi.logger.Warn().Msgf("the account cannot be processed on joltify pub_chain this round with err %v\n", err)
		return nil
	}
	txIDBytes, err := hex.DecodeString(txID)
	if err != nil {
		pi.logger.Warn().Msgf("invalid tx ID %v\n", txIDBytes)
		return nil
	}
	roundBlockHeight := blockHeight / ROUNDBLOCK
	item := bcommon.NewAccountInboundReq(tx.address, to, tx.token, txIDBytes, int64(roundBlockHeight))
	pi.InboundReqChan <- &item
	return nil
}

func (pi *PubChainInstance) checkErc20(data []byte) (common.Address, *big.Int, error) {
	if method, ok := pi.tokenAbi.Methods["transfer"]; ok {
		if len(data) < 4 {
			return common.Address{}, nil, errors.New("invalid data")
		}
		params, err := method.Inputs.Unpack(data[4:])
		if err != nil {
			return common.Address{}, nil, err
		}
		if len(params) != 2 {
			return common.Address{}, nil, errors.New("invalid transfer parameter")
		}
		toAddr, ok := params[0].(common.Address)
		if !ok {
			return common.Address{}, nil, errors.New("not valid address")
		}
		amount, ok := params[1].(*big.Int)
		if !ok {
			return common.Address{}, nil, errors.New("not valid amount")
		}
		return toAddr, amount, nil
	}
	return common.Address{}, nil, errors.New("invalid method for decode")
}

// fixme we need to check timeout to remove the pending transactions
func (pi *PubChainInstance) processEachBlock(block *ethTypes.Block, joltifyBlockHeight int64) {
	for _, tx := range block.Transactions() {
		if tx.To() == nil {
			continue
		}
		status, err := pi.checkEachTx(tx.Hash())
		if err != nil || status != 1 {
			continue
		}

		toAddr, amount, err := pi.checkErc20(tx.Data())
		if err == nil {
			if tx.To().Hex() != pi.tokenAddr {
				// this indicates it is not to our smart contract
				continue
			}
			// process the public chain inbound message to the channel
			if !pi.checkToBridge(toAddr) {
				pi.logger.Warn().Msg("the top up message is not to the bridge, ignored")
				continue
			}
			err := pi.ProcessInBoundERC20(tx, *tx.To(), toAddr, amount, block.NumberU64())
			if err != nil {
				zlog.Logger.Error().Err(err).Msg("fail to process the inbound contract message")
				continue
			}
		}

		if pi.checkToBridge(*tx.To()) {
			if tx.Data() == nil {
				pi.logger.Warn().Msgf("we have received unknown fund")
				continue
			}

			payTxID := tx.Data()
			account := pi.updateInboundTx(hex.EncodeToString(payTxID), tx.Value(), block.NumberU64())
			if account != nil {
				item := bcommon.NewAccountInboundReq(account.address, *tx.To(), account.token, payTxID, joltifyBlockHeight)
				// we add to the retry pool to  sort the tx
				pi.AddItem(&item)
			}
		}
	}
}

// UpdatePool update the tss pool address
func (pi *PubChainInstance) UpdatePool(pool *vaulttypes.PoolInfo) error {
	if pool == nil {
		return errors.New("nil pool")
	}
	poolPubKey := pool.CreatePool.PoolPubKey
	addr, err := misc.PoolPubKeyToJoltAddress(poolPubKey)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to convert the jolt addres to eth address %v", poolPubKey)
		return err
	}

	ethAddr, err := misc.PoolPubKeyToEthAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the jolt address to eth address %v", poolPubKey)
		return err
	}

	pi.poolLocker.Lock()
	defer pi.poolLocker.Unlock()

	p := bcommon.PoolInfo{
		Pk:             poolPubKey,
		JoltifyAddress: addr,
		EthAddress:     ethAddr,
		PoolInfo:       pool,
	}

	if pi.lastTwoPools[1] != nil {
		pi.lastTwoPools[0] = pi.lastTwoPools[1]
	}
	pi.lastTwoPools[1] = &p
	return nil
}

// GetPool get the latest two pool address
func (pi *PubChainInstance) GetPool() []*bcommon.PoolInfo {
	pi.poolLocker.RLock()
	defer pi.poolLocker.RUnlock()
	var ret []*bcommon.PoolInfo
	ret = append(ret, pi.lastTwoPools...)
	return ret
}

// GetPool get the latest two pool address
func (pi *PubChainInstance) checkToBridge(dest common.Address) bool {
	pools := pi.GetPool()
	for _, el := range pools {
		if el != nil && dest.String() == el.EthAddress.String() {
			return true
		}
	}
	return false
}

// DeleteExpired delete the expired tx
func (pi *PubChainInstance) DeleteExpired(currentHeight uint64) {
	var expiredTx []string
	var expiredTxBnb []string
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		el := value.(*inboundTx)
		if currentHeight-el.pubBlockHeight > config.TxTimeout {
			expiredTx = append(expiredTx, key.(string))
		}
		return true
	})

	for _, el := range expiredTx {
		pi.logger.Warn().Msgf("we delete the expired tx %s", el)
		pi.pendingInbounds.Delete(el)
	}

	pi.pendingInboundsBnB.Range(func(key, value interface{}) bool {
		el := value.(*inboundTxBnb)
		if currentHeight-el.blockHeight > config.TxTimeout {
			expiredTxBnb = append(expiredTxBnb, key.(string))
		}
		return true
	})

	for _, el := range expiredTxBnb {
		pi.logger.Warn().Msgf("we delete the expired tx %s in inbound bnb", el)
		pi.pendingInboundsBnB.Delete(el)
	}
}

// Verify is the function  to verify the correctness of the account on joltify_bridge
func (a *inboundTx) Verify() error {
	if a.fee.Denom != config.InBoundDenomFee {
		return fmt.Errorf("invalid inbound fee denom with fee demo : %v and want %v", a.fee.Denom, config.InBoundDenom)
	}
	amount, err := sdk.NewDecFromStr(config.InBoundFeeMin)
	if err != nil {
		return errors.New("invalid minimal inbound fee")
	}
	if a.fee.Amount.LT(sdk.NewIntFromBigInt(amount.BigInt())) {
		return errors.New("the fee is not enough")
	}
	return nil
}

func (pi *PubChainInstance) AddMoveFundItem(pool *bcommon.PoolInfo, height int64) {
	pi.moveFundReq.Store(height, pool)
}

func (pi *PubChainInstance) PopMoveFundItem() (*bcommon.PoolInfo, int64) {
	min := int64(math.MaxInt64)
	pi.moveFundReq.Range(func(key, value interface{}) bool {
		h := key.(int64)
		if h <= min {
			min = h
		}
		return true
	})
	if min < math.MaxInt64 {
		item, _ := pi.moveFundReq.LoadAndDelete(min)
		return item.(*bcommon.PoolInfo), min
	}
	return nil, 0
}

//PopMoveFundItemAfterBlock pop up the item after the given block duration
func (pi *PubChainInstance) PopMoveFundItemAfterBlock(currentBlockHeight int64) (*bcommon.PoolInfo, int64) {
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

func (pi *PubChainInstance) moveBnb(senderPk string, receiver common.Address, amount *big.Int, nonce uint64, blockHeight int64) (string, error) {

	ctx, cancel := context.WithTimeout(context.Background(), config.QueryTimeOut)
	defer cancel()
	chainID, err := pi.EthClient.NetworkID(ctx)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to get the chain ID")
		return "", err
	}

	gasPrice, err := pi.EthClient.SuggestGasPrice(context.Background())
	if err != nil {
		return "", err
	}

	gasLimit, err := pi.EthClient.EstimateGas(context.Background(), ethereum.CallMsg{
		To:   &receiver,
		Data: nil,
	})
	if err != nil {
		return "", err
	}

	totalBnb := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(gasLimit))

	totalBnbDec := sdk.NewDecFromBigIntWithPrec(totalBnb, sdk.Precision)
	totalBnbDec = totalBnbDec.Mul(sdk.MustNewDecFromStr(config.GASFEERATIO))

	moveFund := amount.Sub(amount, totalBnbDec.BigInt())
	moveFundS := sdk.NewDecFromBigIntWithPrec(moveFund, sdk.Precision)
	pi.logger.Info().Msgf("we need to move %v bnb", moveFundS.String())

	dustBnb, err := sdk.NewDecFromStr(config.DUSTBNB)
	if err != nil {
		panic("invalid parameter")
	}

	if moveFund.Cmp(dustBnb.BigInt()) != 1 {
		return "", nil
	}
	baseTx := ethTypes.LegacyTx{
		Nonce:    nonce,
		GasPrice: gasPrice,
		Gas:      gasLimit,
		To:       &receiver,
		Data:     nil,
		Value:    moveFund,
	}

	rawTx := ethTypes.NewTx(&baseTx)
	signer := ethTypes.LatestSignerForChainID(chainID)
	msg := signer.Hash(rawTx).Bytes()
	signature, err := pi.tssSign(msg, senderPk, blockHeight)
	if err != nil || len(signature) != 65 {
		return "", errors.New("fail to get the valid signature")
	}
	bTx, err := rawTx.WithSignature(signer, signature)
	if err != nil {
		return "", err
	}

	err = pi.EthClient.SendTransaction(ctx, bTx)
	if err != nil {
		if err.Error() == "already known" || err.Error() == "replacement transaction underpriced" {
			pi.logger.Warn().Msgf("the tx has been submitted by others")
			return rawTx.Hash().Hex(), nil
		} else {
			return "", err
		}
	}

	return rawTx.Hash().Hex(), nil
}

func (pi *PubChainInstance) moveERC20Token(senderPk string, sender, receiver common.Address, balance *big.Int, blockheight int64) (string, error) {

	txHash, err := pi.SendToken(senderPk, sender, receiver, balance, blockheight, nil)
	if err != nil {
		if err.Error() == "already known" {
			pi.logger.Warn().Msgf("the tx has been submitted by others")
			return txHash.Hex(), nil
		}
		pi.logger.Error().Err(err).Msgf("fail to send the token with err %v for amount %v ", err, balance)
		return "", err
	}

	return txHash.Hex(), nil
}

func (pi *PubChainInstance) MoveFunds(previousPool *bcommon.PoolInfo, receiver common.Address, blockHeight int64) (bool, error) {
	tokenInstance := pi.tokenInstance
	balance, err := tokenInstance.BalanceOf(&bind.CallOpts{}, previousPool.EthAddress)
	if err != nil {
		return false, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.QueryTimeOut)
	defer cancel()
	balanceBnB, err := pi.EthClient.BalanceAt(ctx, previousPool.EthAddress, nil)
	if err != nil {
		return false, err
	}

	dustBnb, err := sdk.NewDecFromStr(config.DUSTBNB)
	if err != nil {
		panic("invalid parameter")
	}

	tick := html.UnescapeString("&#" + "9193" + ";")
	pi.logger.Info().Msgf(" %v we move fund from %v to %v\n", tick, previousPool.EthAddress.String(), receiver.String())

	if balance.Cmp(big.NewInt(0)) == 0 && balanceBnB.Cmp(dustBnb.BigInt()) != 1 {
		return true, nil
	}

	var erc20TxHash, bnbTxHash string
	if balance.Cmp(big.NewInt(0)) == 1 {
		erc20TxHash, err = pi.moveERC20Token(previousPool.Pk, previousPool.EthAddress, receiver, balance, blockHeight)
		//if we fail erc20 token transfer, we should not transfer the bnb otherwise,we do not have enough fee to pay retry
		if err != nil {
			return false, errors.New("fail to transfer erc20 token")
		} else {
			//next round, we will handle bnb transfer
			return false, nil
		}
	} else {
		pi.logger.Warn().Msg("0 ERC20 balance do not need to move as")
	}

	//now we move the bnb

	if balanceBnB.Cmp(big.NewInt(0)) == 1 {
		//we move the bnb
		nonce, err := pi.EthClient.NonceAt(context.Background(), previousPool.EthAddress, nil)
		if err != nil {
			return false, err
		}
		bnbTxHash, err = pi.moveBnb(previousPool.Pk, receiver, balanceBnB, nonce, blockHeight)
		if err != nil {
			return false, err
		}
	}

	tick = html.UnescapeString("&#" + "127974" + ";")
	zlog.Logger.Info().Msgf(" %v we have moved the fund in the publicchain with tx (ERC20): %v, (BNB): %v", tick, erc20TxHash, bnbTxHash)

	return false, nil
}

func (pi *PubChainInstance) checkEachTx(h common.Hash) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.QueryTimeOut)
	defer cancel()
	receipt, err := pi.EthClient.TransactionReceipt(ctx, h)
	if err != nil {
		return 0, err
	}
	return receipt.Status, nil
}

//CheckTxStatus check whether the tx is already in the chain
func (pi *PubChainInstance) CheckTxStatus(hashStr string) error {

	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(submitBackoff), 40)

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
