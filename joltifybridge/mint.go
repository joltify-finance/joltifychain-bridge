package joltifybridge

import (
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/pubchain"
	vaulttypes "gitlab.com/joltify/joltifychain/joltifychain/x/vault/types"
)

func (jc *JoltifyChainBridge) MintCoin(item *pubchain.AccountInboundReq) error {
	userAcc, poolAddr, coin := item.GetInboundReqInfo()

	creator, err := misc.EthAddressToJoltAddr(poolAddr)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to get the pool address")
		return err
	}

	receiver, err := misc.EthAddressToJoltAddr(userAcc)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to get the user address")
		return err
	}

	a := vaulttypes.IssueToken{Creator: creator, Index: "1", Coin: &coin, Receiver: receiver}
	_ = a
	return nil
}
