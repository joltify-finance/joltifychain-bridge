package pubchain

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	zlog "github.com/rs/zerolog/log"

	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"

	"github.com/cosmos/cosmos-sdk/types"

	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

const alreadyKnown = "already known"

// ProcessInBoundERC20 process the inbound contract token top-up
func (pi *Instance) ProcessInBoundERC20(tx *ethTypes.Transaction, chainType string, txInfo *Erc20TxInfo, txBlockHeight uint64) error {
	err := pi.processInboundERC20Tx(tx.Hash().Hex()[2:], chainType, txBlockHeight, txInfo.receiverAddr, txInfo.tokenAddress, txInfo.Amount, txInfo.tokenAddress)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to process the inbound tx")
		return err
	}
	return nil
}

// ProcessNewBlock process the blocks received from the public pub_chain
func (pi *Instance) ProcessNewBlock(chainType string, chainInfo *ChainInfo, number *big.Int) error {
	block, err := chainInfo.GetBlockByNumberWithLock(number)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to retrieve the block")
		return err
	}
	// we need to put the block height in which we find the tx
	pi.processEachBlock(chainType, chainInfo, block, number.Int64())
	return nil
}

func (pi *Instance) processInboundERC20Tx(txID, chainType string, txBlockHeight uint64, receiverAddr types.AccAddress, to common.Address, value *big.Int, addr common.Address) error {
	// this is repeated check for tokenAddr which is cheked at function 'processEachBlock'
	tokenItem, exit := pi.TokenList.GetTokenInfoByAddressAndChainType(strings.ToLower(addr.Hex()), chainType)
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
		receiverAddr,
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

	item := bcommon.NewAccountInboundReq(tx.ReceiverAddress, to, tx.Token, txIDBytes, int64(txBlockHeight))
	pi.AddInBoundItem(&item)
	return nil
}

func (pi *Instance) checkErc20(data []byte, to, contractAddress string) (*Erc20TxInfo, error) {
	// address toAddress, uint256 amount, address contractAddress, bytes memo

	// check it is from our smart contract
	if !strings.EqualFold(to, contractAddress) {
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
			pi.logger.Error().Err(err).Msgf("unable to unmarshal")
			return nil, err
		}

		var dstAddr types.AccAddress
		var dstAddrErc20 string
		switch memoInfo.ChainType {
		case OPPY:
			dstAddr, err = types.AccAddressFromBech32(memoInfo.Dest)
			if err != nil {
				return nil, err
			}
			dstAddrErc20 = ""
		case BSC, ETH:
			dstAddr = types.AccAddress{}
			dstAddrErc20 = memoInfo.Dest
		default:
			return nil, errors.New("unknow chain type")
		}

		ret := Erc20TxInfo{
			receiverAddr:      dstAddr,
			toAddr:            toAddr,
			Amount:            amount,
			tokenAddress:      tokenAddress,
			dstChainType:      memoInfo.ChainType,
			receiverAddrERC20: dstAddrErc20,
		}

		return &ret, nil
	}
	return nil, errors.New("invalid method for decode")
}

func (pi *Instance) processEachBlock(chainType string, chainInfo *ChainInfo, block *ethTypes.Block, txBlockHeight int64) {
	for _, tx := range block.Transactions() {
		if tx.To() == nil {
			continue
		}
		status, err := chainInfo.checkEachTx(tx.Hash())
		if err != nil || status != 1 {
			continue
		}
		txInfo, err := pi.checkErc20(tx.Data(), tx.To().Hex(), chainInfo.contractAddress)
		if err == nil {
			_, exit := pi.TokenList.GetTokenInfoByAddressAndChainType(txInfo.tokenAddress.String(), chainType)
			if !exit {
				// this indicates it is not to our smart contract
				continue
			}
			// process the public chain inbound message to the channel
			if !pi.checkToBridge(txInfo.toAddr) {
				pi.logger.Warn().Msg("the top up message is not to the bridge, ignored")
				continue
			}

			switch txInfo.dstChainType {
			case "ETH", "BSC":
				err := pi.processDstInbound(txInfo, tx.Hash().Hex()[2:], chainType, txBlockHeight)
				if err != nil {
					zlog.Logger.Error().Err(err).Msgf("fail to process the inbound tx for outbound from %v to %v", chainType, txInfo.dstChainType)
					continue
				}

			case "OPPY":
				err := pi.ProcessInBoundERC20(tx, chainType, txInfo, block.NumberU64())
				if err != nil {
					zlog.Logger.Error().Err(err).Msg("fail to process the inbound contract message")
					continue
				}
			default:
				zlog.Warn().Msgf("fail to process the tx with chain type %v", txInfo.dstChainType)
				continue
			}
		}
		if pi.checkToBridge(*tx.To()) {
			var memoInfo bcommon.BridgeMemo
			err = json.Unmarshal(tx.Data(), &memoInfo)
			if err != nil {
				pi.logger.Error().Err(err).Msgf("fail to unmarshal the memo")
				continue
			}
			switch memoInfo.ChainType {
			case "OPPY":
				pi.processOppyInbound(memoInfo, chainType, *tx, txBlockHeight)
			default:
				pi.logger.Warn().Msgf("unknown chain type %v", memoInfo.ChainType)
			}
		}
	}
}

func calculateFee(a types.Int, ratio string) (types.Int, types.Int) {
	amountDec := a.ToDec()
	leftOver := amountDec.Mul(types.MustNewDecFromStr(ratio)).TruncateInt()
	fee := a.Sub(leftOver)
	return leftOver, fee
}

func (pi *Instance) processDstInbound(txInfo *Erc20TxInfo, txHash, chainType string, txBlockHeight int64) error {
	tokenInItem, exit := pi.TokenList.GetTokenInfoByAddressAndChainType(strings.ToLower(txInfo.tokenAddress.Hex()), chainType)
	if !exit {
		pi.logger.Error().Msgf("Token is not on our token list")
		return errors.New("token is not on our token list")
	}

	token := types.NewCoin(tokenInItem.Denom, types.NewIntFromBigInt(txInfo.Amount))

	delta := types.Precision - tokenInItem.Decimals
	if delta != 0 {
		adjustedTokenAmount := bcommon.AdjustInt(token.Amount, int64(delta))
		token.Amount = adjustedTokenAmount
	}

	//fixme we need to have the dynamic fee
	leftover, fee := calculateFee(token.Amount, "0.9")
	if leftover.IsZero() {
		pi.logger.Warn().Msg("zero value to be transffered")
		return errors.New("zero value to be transferred, rejected")
	}
	feeToValidator := types.NewCoin(token.Denom, fee)
	token.Amount = leftover

	tokenOutItem, tokenExist := pi.TokenList.GetTokenInfoByDenomAndChainType(token.Denom, txInfo.dstChainType)
	if !tokenExist {
		pi.logger.Error().Msgf("fail to find the token %v for outbound chain %v", token.Denom, txInfo.dstChainType)
		return errors.New("cannot find the token")
	}

	//now outbound decimal convertion
	deltaOut := tokenOutItem.Decimals - types.Precision
	if deltaOut != 0 {
		adjustedTokenAmount := bcommon.AdjustInt(token.Amount, int64(deltaOut))
		token.Amount = adjustedTokenAmount
	}

	receiver := common.HexToAddress(txInfo.receiverAddrERC20)
	if receiver == common.BigToAddress(big.NewInt(0)) {
		pi.logger.Warn().Msgf("incorrect receiver address %v for chain %v", txInfo.receiverAddrERC20, txInfo.dstChainType)
		return nil
	}
	currEthAddr := pi.lastTwoPools[1].EthAddress

	itemReq := bcommon.NewOutboundReq(txHash, receiver, currEthAddr, token, tokenOutItem.TokenAddr, txBlockHeight, types.Coins{}, types.Coins{feeToValidator}, txInfo.dstChainType)

	pi.AddOutBoundItem(&itemReq)
	pi.logger.Info().Msgf("Outbound Transaction in Block %v (Current Block %v) with fee %v paid to validators", txBlockHeight, pi.CurrentHeight, types.Coins{feeToValidator})
	return nil
}

func (pi *Instance) processOppyInbound(memoInfo bcommon.BridgeMemo, chainType string, tx ethTypes.Transaction, txBlockHeight int64) {

	dstAddr, err := types.AccAddressFromBech32(memoInfo.Dest)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to the acc address")
		return
	}

	tokenItem, exist := pi.TokenList.GetTokenInfoByAddressAndChainType("native", chainType)
	if !exist {
		panic("native token is not set")
	}
	// this indicates it is a native bnb transfer
	balance, err := pi.getBalance(tx.Value(), tokenItem.Denom)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to the the balance of the given token")
		return
	}
	delta := types.Precision - tokenItem.Decimals
	if delta != 0 {
		adjustedTokenAmount := bcommon.AdjustInt(balance.Amount, int64(delta))
		balance.Amount = adjustedTokenAmount
	}

	item := bcommon.NewAccountInboundReq(dstAddr, *tx.To(), balance, tx.Hash().Bytes(), txBlockHeight)
	// we add to the retry pool to  sort the tx
	pi.AddInBoundItem(&item)

}

// UpdatePool update the tss pool address
func (pi *Instance) UpdatePool(pool *vaulttypes.PoolInfo) error {
	if pool == nil {
		return errors.New("nil pool")
	}
	poolPubKey := pool.CreatePool.PoolPubKey
	addr, err := misc.PoolPubKeyToOppyAddress(poolPubKey)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to convert the joltify address to eth address %v", poolPubKey)
		return err
	}

	ethAddr, err := misc.PoolPubKeyToEthAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the joltify address to eth address %v", poolPubKey)
		return err
	}

	pi.poolLocker.Lock()
	defer pi.poolLocker.Unlock()

	p := bcommon.PoolInfo{
		Pk:         poolPubKey,
		CosAddress: addr,
		EthAddress: ethAddr,
		PoolInfo:   pool,
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
