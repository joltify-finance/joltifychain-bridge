package cosbridge

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

func (jc *JoltChainInstance) processOutBoundRequest(msg *banktypes.MsgSend, txID string, txBlockHeight int64, currEthAddr ethcommon.Address, memo bcommon.BridgeMemo) error {
	tokenItem, tokenExist := jc.TokenList.GetTokenInfoByDenomAndChainType(msg.Amount[0].GetDenom(), memo.ChainType)
	if !tokenExist {
		return errors.New("token is not on our token list")
	}

	item := jc.processDemonAndFee(txID, msg.FromAddress, tokenItem.TokenAddr, txBlockHeight, ethcommon.HexToAddress(memo.Dest), msg.Amount[0].GetDenom(), msg.Amount[0].Amount, memo.ChainType)
	// since the cosmos address is different from the eth address, we need to derive the eth address from the public key
	if item != nil {
		item.Token.Amount = outboundAdjust(item.Token.Amount, tokenItem.Decimals, types.Precision)
		itemReq := bcommon.NewOutboundReq(txID, item.OutReceiverAddress, currEthAddr, item.Token, tokenItem.TokenAddr, txBlockHeight, types.Coins{item.Fee}, memo.ChainType, false)
		jc.AddItem(&itemReq)
		jc.logger.Info().Msgf("Outbound Transaction in Block %v (Current Block %v) with fee %v paid to validators", txBlockHeight, jc.CurrentHeight, types.Coins{item.Fee})
		return nil
	}
	return nil
}

// processMsg handle the oppychain transactions
func (jc *JoltChainInstance) processMsg(txBlockHeight int64, address []types.AccAddress, curEthAddr ethcommon.Address, memo bcommon.BridgeMemo, msg *banktypes.MsgSend, txHash []byte) error {
	if msg.Amount.IsZero() {
		return errors.New("zero amount")
	}
	txID := strings.ToLower(hex.EncodeToString(txHash))

	toAddress, err := types.AccAddressFromBech32(msg.ToAddress)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to parse the to outReceiverAddress")
		return err
	}

	// we check whether it is the message to the pool
	if !(toAddress.Equals(address[0]) || toAddress.Equals(address[1])) {
		jc.logger.Warn().Msg("not a top up message to the pool")
		return errors.New("not a top up message to the pool")
	}

	err = jc.processOutBoundRequest(msg, txID, txBlockHeight, curEthAddr, memo)
	if err != nil {
		return fmt.Errorf("fail to process the outbound erc20 request %w", err)
	}
	return nil
}

func (jc *JoltChainInstance) processDemonAndFee(txID, fromAddress string, tokenAddr string, blockHeight int64, receiverAddr ethcommon.Address, demonName string, demonAmount types.Int, chainType string) *OutboundTx {
	token := types.Coin{
		Denom:  demonName,
		Amount: demonAmount,
	}

	feeModule, ok := jc.FeeModule[chainType]
	if !ok {
		panic("the fee module does not exist!!")
	}

	jc.grpcLock.Lock()
	price, err := QueryTokenPrice(jc.GrpcClient, jc.grpcAddr, token.GetDenom())
	jc.grpcLock.Unlock()
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to get the token price")
		return nil
	}

	fee, err := bcommon.CalculateFee(feeModule, price, token)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to calculate the fee")
		return nil
	}

	if token.IsLT(fee) || token.Equal(fee) {
		jc.logger.Warn().Msg("token is smaller than the fee,we drop the tx")
		return nil
	}
	token = token.Sub(fee)

	tx := OutboundTx{
		receiverAddr,
		fromAddress,
		// todo should be dynamic once we have another chain
		uint64(blockHeight),
		token,
		tokenAddr,
		fee,
		txID,
		chainType,
	}

	jc.logger.Info().Msgf("we add the outbound tokens tx(%v):%v(fee: %v)", txID, tx.Token.String(), fee.String())
	return &tx
}

// GetPool get the latest two pool address
func (jc *JoltChainInstance) GetPool() []*bcommon.PoolInfo {
	jc.poolUpdateLocker.RLock()
	defer jc.poolUpdateLocker.RUnlock()
	var ret []*bcommon.PoolInfo
	ret = append(ret, jc.lastTwoPools...)
	return ret
}

// UpdatePool update the tss pool address
func (jc *JoltChainInstance) UpdatePool(pool *vaulttypes.PoolInfo) *bcommon.PoolInfo {
	poolPubKey := pool.CreatePool.PoolPubKey
	ethAddr, err := misc.PoolPubKeyToEthAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the joltify address to eth address %v", poolPubKey)
		return nil
	}

	addr, err := misc.PoolPubKeyToOppyAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the eth address to joltify address %v", poolPubKey)
		return nil
	}

	p := bcommon.PoolInfo{
		Pk:         poolPubKey,
		CosAddress: addr,
		EthAddress: ethAddr,
		PoolInfo:   pool,
	}

	jc.poolUpdateLocker.Lock()
	previousPool := jc.lastTwoPools[0]

	if jc.lastTwoPools[1] != nil {
		jc.lastTwoPools[0] = jc.lastTwoPools[1]
	}
	jc.lastTwoPools[1] = &p
	jc.poolUpdateLocker.Unlock()
	return previousPool
}
