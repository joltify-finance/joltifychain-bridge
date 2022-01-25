package joltifybridge

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	zlog "github.com/rs/zerolog/log"

	bcommon "gitlab.com/joltify/joltifychain-bridge/common"

	"github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

func (jc *JoltifyChainBridge) processMsg(blockHeight int64, address []types.AccAddress, curEthAddr ethcommon.Address, msg *banktypes.MsgSend, txHash []byte, memo string) error {
	txID := strings.ToLower(hex.EncodeToString(txHash))
	_, ok := jc.pendingOutbounds.Load(txID)
	if ok {
		jc.logger.Error().Msgf("the tx already exist!!")
		return errors.New("tx existed")
	}

	toAddress, err := types.AccAddressFromBech32(msg.ToAddress)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to parse the to outReceiverAddress")
		return err
	}

	fromAddress, err := types.AccAddressFromBech32(msg.FromAddress)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to parse the from outReceiverAddress")
		return err
	}

	// here we need to calculate the node's eth address from public key rather than the joltify chain address
	acc, err := queryAccount(msg.FromAddress, jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to query the account")
		return err
	}

	fromEthAddr, err := misc.AccountPubKeyToEthAddress(acc.GetPubKey())
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to get the eth address")
		return err
	}
	// now we wrap the fromEthAddress with joltify hex address
	wrapFromEthAddr, err := types.AccAddressFromHex(fromEthAddr.Hex()[2:])
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to wrap the eth address")
		return err
	}

	// we check whether we are
	if !(toAddress.Equals(address[0]) || toAddress.Equals(address[1])) {
		jc.logger.Warn().Msg("not a top up message to the pool")
		return nil
	}

	// it means the sender pay the fee in one tx
	if len(msg.Amount) == 2 {
		// now we search for the index of the outboundemo and the outbounddemofee
		found := false
		indexDemo := 0
		indexDemoFee := 0
		if msg.Amount[0].GetDenom() == config.OutBoundDenom && msg.Amount[1].GetDenom() == config.OutBoundDenomFee {
			indexDemo = 0
			indexDemoFee = 1
			found = true
		}

		if msg.Amount[1].GetDenom() == config.OutBoundDenom && msg.Amount[0].GetDenom() == config.OutBoundDenomFee {
			indexDemo = 1
			indexDemoFee = 0
			found = true
		}
		if !found {
			return errors.New("invalid fee pair")
		}

		jc.processDemon(txID, blockHeight, wrapFromEthAddr, msg.Amount[indexDemo].Amount)

		item := jc.processFee(txID, msg.Amount[indexDemoFee].Amount)
		// since the cosmos address is different from the eth address, we need to derive the eth address from the public key
		if item != nil {
			itemReq := newAccountOutboundReq(item.outReceiverAddress, curEthAddr, item.token, blockHeight)
			jc.OutboundReqChan <- &itemReq
		}
		return nil
	}

	for _, el := range msg.Amount {
		switch el.Denom {
		case config.OutBoundDenom:
			fmt.Printf("process %v\n", el.Denom)
			amount := msg.Amount.AmountOf(config.OutBoundDenom)
			jc.processDemon(txID, blockHeight, fromAddress, amount)
		case config.OutBoundDenomFee:
			fmt.Printf("process %v\n", el.Denom)
			amount := msg.Amount.AmountOf(config.OutBoundDenomFee)
			item := jc.processFee(memo, amount)
			if item != nil {
				itemReq := newAccountOutboundReq(item.outReceiverAddress, ethcommon.BytesToAddress(fromAddress.Bytes()), item.token, blockHeight)
				jc.OutboundReqChan <- &itemReq
			}
		default:
			jc.logger.Warn().Msg("unknown token")
			return nil

		}
	}

	return nil
}

func (jc *JoltifyChainBridge) processFee(txID string, amount types.Int) *outboundTx {
	data, ok := jc.pendingOutbounds.Load(strings.ToLower(txID))
	if !ok {
		jc.logger.Warn().Msgf("fail to get the stored tx from pool with %v\n", jc.pendingOutbounds)
		return nil
	}
	thisAccount := data.(*outboundTx)
	thisAccount.fee.Amount = thisAccount.fee.Amount.Add(amount)
	err := thisAccount.Verify()
	if err != nil {
		jc.pendingOutbounds.Store(txID, thisAccount)
		jc.logger.Warn().Err(err).Msgf("the account cannot be processed on joltify pub_chain this round")
		return nil
	}
	// since this tx is processed,we do not need to store it any longer
	jc.pendingOutbounds.Delete(txID)
	return thisAccount
}

func (jc *JoltifyChainBridge) processDemon(txID string, blockHeight int64, fromAddress types.AccAddress, amount types.Int) {
	token := types.Coin{
		Denom:  config.OutBoundDenom,
		Amount: amount,
	}
	fee := types.Coin{
		Denom:  config.OutBoundDenomFee,
		Amount: types.NewInt(0),
	}

	tx := outboundTx{
		ethcommon.BytesToAddress(fromAddress.Bytes()),
		uint64(blockHeight),
		token,
		fee,
	}
	jc.logger.Info().Msgf("we add the outbound tokens tx(%v):%v", txID, tx.token.String())
	jc.pendingOutbounds.Store(txID, &tx)
}

// GetPool get the latest two pool address
func (jc *JoltifyChainBridge) GetPool() []*bcommon.PoolInfo {
	jc.poolUpdateLocker.RLock()
	defer jc.poolUpdateLocker.RUnlock()
	var ret []*bcommon.PoolInfo
	ret = append(ret, jc.lastTwoPools...)
	return ret
}

// UpdatePool update the tss pool address
func (jc *JoltifyChainBridge) UpdatePool(poolPubKey string) {
	ethAddr, err := misc.PoolPubKeyToEthAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the jolt address to eth address %v", poolPubKey)
		return
	}

	addr, err := misc.PoolPubKeyToJoltAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the jolt address to jolt address %v", poolPubKey)
		return
	}

	p := bcommon.PoolInfo{
		Pk:             poolPubKey,
		JoltifyAddress: addr,
		EthAddress:     ethAddr,
	}
	query := fmt.Sprintf("tm.event = 'Tx' AND transfer.recipient= '%s'", p.JoltifyAddress.String())
	out, err := jc.wsClient.Subscribe(context.Background(), p.JoltifyAddress.String(), query)
	if err != nil {
		zlog.Logger.Error().Err(err).Msg("fail to subscribe the new transfer pool address")
	}

	jc.poolUpdateLocker.Lock()
	defer jc.poolUpdateLocker.Unlock()
	// jc.transferChanUpdateLocker.Lock()
	// defer jc.transferChanUpdateLocker.Unlock()
	if jc.lastTwoPools[1] != nil {
		if jc.lastTwoPools[0] != nil && jc.lastTwoPools[0].JoltifyAddress.String() != p.JoltifyAddress.String() {
			delQuery := fmt.Sprintf("tm.event = 'Tx' AND transfer.recipient= '%s'", jc.lastTwoPools[0].JoltifyAddress.String())
			err := jc.wsClient.Unsubscribe(context.Background(), "quitQuery", delQuery)
			if err != nil {
				jc.logger.Error().Err(err).Msgf("fail to unsubscribe the address %v", err)
			}
		}
		jc.lastTwoPools[0] = jc.lastTwoPools[1]
		jc.TransferChan[0] = jc.TransferChan[1]
	}
	jc.lastTwoPools[1] = &p
	jc.TransferChan[1] = &out
}
