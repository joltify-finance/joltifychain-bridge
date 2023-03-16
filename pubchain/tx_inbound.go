package pubchain

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	types2 "github.com/tendermint/tendermint/proto/tendermint/types"

	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	zlog "github.com/rs/zerolog/log"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"

	"github.com/cosmos/cosmos-sdk/types"

	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

const alreadyKnown = "already known"

// ProcessInBoundERC20 process the inbound contract token top-up
func (pi *Instance) ProcessInBoundERC20(tx *ethTypes.Transaction, chainType string, txInfo *Erc20TxInfo, txBlockHeight uint64) error {
	err := pi.processInboundERC20Tx(tx.Hash().Hex()[2:], chainType, txBlockHeight, txInfo.receiverAddr, txInfo.Amount, txInfo.tokenAddress)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to process the inbound tx")
		return err
	}
	return nil
}

// ProcessNewERC20Block process the blocks received from the public pub_chain
func (pi *Instance) ProcessNewERC20Block(chainType string, chainInfo *Erc20ChainInfo, number *big.Int, feeModule map[string]*bcommon.FeeModule, oppyGrpc string) error {
	block, err := chainInfo.GetBlockByNumberWithLock(number)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to retrieve the block")
		return err
	}
	// we need to put the block height in which we find the tx
	pi.processEachERC20Block(chainType, chainInfo, block, number.Int64(), feeModule, oppyGrpc)
	return nil
}

func (pi *Instance) ProcessNewCosmosBlock(block *types2.Block, pools []*bcommon.PoolInfo, cosmosBlockHeight int64) {
	for _, el := range block.Data.Txs {
		items, err := pi.CosChain.processEachCosmosTx(el, pools, cosmosBlockHeight)
		if err != nil {
			pi.logger.Error().Err(err).Msg("fail to process each cosmos tx")
			continue
		}
		for _, el := range items {
			pi.AddInBoundItem(el)
		}
	}
}

func (pi *Instance) processInboundERC20Tx(txID, chainType string, txBlockHeight uint64, receiverAddr types.AccAddress, value *big.Int, addr common.Address) error {
	// this is repeated check for tokenAddr which is cheked at function 'processEachERC20Block'
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

	tx.Token.Amount = inboundAdjust(tx.Token.Amount, tokenItem.Decimals, types.Precision)
	item := bcommon.NewAccountInboundReq(tx.ReceiverAddress, tx.Token, txIDBytes, int64(txBlockHeight))
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
		case JOLTIFY:
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

func (pi *Instance) processEachERC20Block(chainType string, chainInfo *Erc20ChainInfo, block *ethTypes.Block, txBlockHeight int64, feeModule map[string]*bcommon.FeeModule, oppyGrpc string) {
	for _, tx := range block.Transactions() {
		if tx.To() == nil {
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
				pi.logger.Warn().Msg("tx is not to the bridge, ignored")
				continue
			}
			if txInfo.dstChainType == chainType {
				zlog.Error().Msgf("cannot transfer from and to the same chain")
				continue
			}

			switch txInfo.dstChainType {
			case ETH, BSC:

				status, err := chainInfo.checkEachTx(tx.Hash())
				if err != nil || status != 1 {
					continue
				}
				err = pi.processDstInbound(txInfo, tx.Hash().Hex()[2:], chainType, txBlockHeight, feeModule, oppyGrpc)
				if err != nil {
					zlog.Logger.Error().Err(err).Msgf("fail to process the inbound tx for outbound from %v to %v", chainType, txInfo.dstChainType)
					continue
				}

			case JOLTIFY:

				status, err := chainInfo.checkEachTx(tx.Hash())
				if err != nil || status != 1 {
					continue
				}
				err = pi.ProcessInBoundERC20(tx, chainType, txInfo, block.NumberU64())
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
			case JOLTIFY:

				status, err := chainInfo.checkEachTx(tx.Hash())
				if err != nil || status != 1 {
					continue
				}

				pi.processJoltifyInbound(memoInfo, chainType, *tx, txBlockHeight)
			default:
				pi.logger.Warn().Msgf("unknown chain type %v", memoInfo.ChainType)
			}
		}
	}
}

func (pi *Instance) processDstInbound(txInfo *Erc20TxInfo, txHash, chainType string, txBlockHeight int64, feeModule map[string]*bcommon.FeeModule, oppyGrpc string) error {
	tokenInItem, exit := pi.TokenList.GetTokenInfoByAddressAndChainType(strings.ToLower(txInfo.tokenAddress.Hex()), chainType)
	if !exit {
		pi.logger.Error().Msgf("Token is not on our token list")
		return errors.New("token is not on our token list")
	}

	token := types.NewCoin(tokenInItem.Denom, types.NewIntFromBigInt(txInfo.Amount))
	token.Amount = inboundAdjust(token.Amount, tokenInItem.Decimals, types.Precision)

	price, err := pi.joltHandler.queryTokenPrice(nil, oppyGrpc, token.Denom)
	if err != nil {
		return errors.New("fail to get the token price")
	}

	thisFeeModule, ok := feeModule[txInfo.dstChainType]
	if !ok {
		panic("the fee module does not exist!!")
	}

	fee, err := bcommon.CalculateFee(thisFeeModule, price, token)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to calculate the fee")
		return nil
	}
	if token.IsLT(fee) || token.Equal(fee) {
		pi.logger.Warn().Msg("token is smaller than the fee,we drop the tx")
		return nil
	}

	feeToValidator := fee
	token = token.Sub(feeToValidator)

	tokenOutItem, tokenExist := pi.TokenList.GetTokenInfoByDenomAndChainType(token.Denom, txInfo.dstChainType)
	if !tokenExist {
		pi.logger.Error().Msgf("fail to find the token %v for outbound chain %v", token.Denom, txInfo.dstChainType)
		return errors.New("cannot find the token")
	}

	token.Amount = outboundAdjust(token.Amount, tokenOutItem.Decimals, types.Precision)

	receiver := common.HexToAddress(txInfo.receiverAddrERC20)
	if receiver == common.BigToAddress(big.NewInt(0)) {
		pi.logger.Warn().Msgf("incorrect receiver address %v for chain %v", txInfo.receiverAddrERC20, txInfo.dstChainType)
		return nil
	}
	currEthAddr := pi.lastTwoPools[1].EthAddress

	joltHeight, err := pi.joltHandler.QueryJoltBlockHeight(oppyGrpc)
	if err != nil {
		return errors.New("fail to get the token price")
	}

	itemReq := bcommon.NewOutboundReq(txHash, receiver.Bytes(), currEthAddr.Bytes(), token, tokenOutItem.TokenAddr, joltHeight, types.Coins{feeToValidator}, txInfo.dstChainType, true)
	pi.AddOutBoundItem(&itemReq)
	pi.logger.Info().Msgf("Outbound Transaction in Joltify Block %v  with fee %v paid to validators", txBlockHeight, types.Coins{feeToValidator})
	return nil
}

func (pi *Instance) processJoltifyInbound(memoInfo bcommon.BridgeMemo, chainType string, tx ethTypes.Transaction, txBlockHeight int64) {
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
	balance.Amount = inboundAdjust(balance.Amount, tokenItem.Decimals, types.Precision)
	item := bcommon.NewAccountInboundReq(dstAddr, balance, tx.Hash().Bytes(), txBlockHeight)
	// we add to the retry pool to  sort the tx
	pi.AddInBoundItem(&item)
}

// UpdatePool update the tss pool address
func (pi *Instance) UpdatePool(pool *vaulttypes.PoolInfo) error {
	if pool == nil {
		return errors.New("nil pool")
	}
	poolPubKey := pool.CreatePool.PoolPubKey
	addr, err := misc.PoolPubKeyToJoltifyAddress(poolPubKey)
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
