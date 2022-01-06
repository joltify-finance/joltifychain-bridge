package joltifybridge

import (
	"encoding/hex"
	"errors"
	"fmt"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	"strings"

	"github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

func (jc *JoltifyChainBridge) processMsg(blockHeight int64, address []ethcommon.Address, curEthAddr ethcommon.Address, msg *banktypes.MsgSend, txHash []byte, memo string) error {
	txID := strings.ToLower(hex.EncodeToString(txHash))
	jc.pendingOutboundLocker.Lock()
	_, ok := jc.pendingOutbounds[txID]
	jc.pendingOutboundLocker.Unlock()
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

	currentPoolAddr, err1 := types.AccAddressFromHex(address[0].Hex()[2:])
	privPoolAddr, err2 := types.AccAddressFromHex(address[1].Hex()[2:])
	if err1 != nil || err2 != nil {
		jc.logger.Error().Msgf("fail to parse the pool outReceiverAddress with err1:%v err2:%v", err1, err2)
		return err
	}

	// we check whether we are
	if !(toAddress.Equals(currentPoolAddr) || toAddress.Equals(privPoolAddr)) {
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

		jc.processDemon(txID, blockHeight, fromAddress, msg.Amount[indexDemo].Amount)

		item := jc.processFee(txID, msg.Amount[indexDemoFee].Amount)
		//since the cosmos address is different from the eth address, we need to derive the eth address from the public key
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
	jc.pendingOutboundLocker.Lock()
	defer jc.pendingOutboundLocker.Unlock()
	thisAccount, ok := jc.pendingOutbounds[strings.ToLower(txID)]
	if !ok {
		jc.logger.Warn().Msgf("fail to get the stored tx from pool with %v\n", jc.pendingOutbounds)
		return nil
	}

	thisAccount.fee.Amount = thisAccount.fee.Amount.Add(amount)
	err := thisAccount.Verify()
	if err != nil {
		jc.pendingOutbounds[txID] = thisAccount
		jc.logger.Warn().Err(err).Msgf("the account cannot be processed on joltify pub_chain this round")
		return nil
	}
	// since this tx is processed,we do not need to store it any longer
	delete(jc.pendingOutbounds, txID)
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
	jc.poolUpdateLocker.Lock()
	jc.pendingOutbounds[txID] = &tx
	jc.poolUpdateLocker.Unlock()
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
	addr, err := misc.PoolPubKeyToEthAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the jolt address to eth address %v", poolPubKey)
		return
	}
	jc.poolUpdateLocker.Lock()
	defer jc.poolUpdateLocker.Unlock()

	p := bcommon.PoolInfo{
		Pk:      poolPubKey,
		Address: addr,
	}

	if jc.lastTwoPools[1] != nil {
		jc.lastTwoPools[0] = jc.lastTwoPools[1]
	}
	jc.lastTwoPools[1] = &p
	return
}
