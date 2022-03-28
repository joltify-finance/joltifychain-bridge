package joltifybridge

import (
	"errors"
	zlog "github.com/rs/zerolog/log"
	"gitlab.com/joltify/joltifychain-bridge/pubchain"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

//FeedTx feed the tx with the given
func (jc *JoltifyChainInstance) FeedTx(lastPoolInfo *vaulttypes.PoolInfo, pi *pubchain.Instance, currentBlockHeight int64) error {

	// we always increase the account seq regardless the tx successful or not
	currentPool := lastPoolInfo.CreatePool.PoolAddr
	acc, err := queryAccount(currentPool.String(), jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msgf("fail to query the account")
		return errors.New("invalid account query")
	}

	inboundItems := pi.PopItem(pubchain.GroupSign)
	if inboundItems == nil {
		jc.logger.Info().Msgf("empty queue")
		return nil
	}
	roundBlockHeight := currentBlockHeight / ROUNDBLOCK

	accNum := acc.GetAccountNumber()
	accSeq := acc.GetSequence()
	address := acc.GetAddress()
	poolPubkey := lastPoolInfo.CreatePool.PoolPubKey
	for _, el := range inboundItems {
		found, err := jc.CheckWhetherSigner(lastPoolInfo)
		if err != nil {
			zlog.Logger.Error().Err(err).Msg("fail to check whether we are the node submit the mint request")
			continue
		}
		if !found {
			continue
		}
		el.SetItemHeight(roundBlockHeight)
		el.SetAccountInfo(accNum, accSeq, address, poolPubkey)
		accSeq += 1
		pi.InboundReqChan <- el
	}
	return nil
}
