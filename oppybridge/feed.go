package oppybridge

import (
	"errors"

	zlog "github.com/rs/zerolog/log"
	"gitlab.com/oppy-finance/oppy-bridge/pubchain"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
)

// FeedTx feed the tx with the given
func (oc *OppyChainInstance) FeedTx(lastPoolInfo *vaulttypes.PoolInfo, pi *pubchain.Instance, currentBlockHeight int64) error {
	// we always increase the account seq regardless the tx successful or not
	currentPool := lastPoolInfo.CreatePool.PoolAddr
	acc, err := queryAccount(currentPool.String(), oc.grpcClient)
	if err != nil {
		oc.logger.Error().Err(err).Msgf("fail to query the account")
		return errors.New("invalid account query")
	}

	inboundItems := pi.PopItem(pubchain.GroupSign)
	if inboundItems == nil {
		oc.logger.Info().Msgf("empty queue")
		return nil
	}
	roundBlockHeight := currentBlockHeight / ROUNDBLOCK

	accNum := acc.GetAccountNumber()
	accSeq := acc.GetSequence()
	address := acc.GetAddress()
	poolPubkey := lastPoolInfo.CreatePool.PoolPubKey
	for _, el := range inboundItems {
		found, err := oc.CheckWhetherSigner(lastPoolInfo)
		if err != nil {
			zlog.Logger.Error().Err(err).Msg("fail to check whether we are the node submit the mint request")
			continue
		}
		if !found {
			zlog.Info().Msgf("we are not the signer")
			continue
		}

		el.SetAccountInfoAndHeight(accNum, accSeq, address, poolPubkey, roundBlockHeight)
		accSeq += 1
		pi.InboundReqChan <- el
	}
	return nil
}
