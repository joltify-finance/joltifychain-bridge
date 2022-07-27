package oppybridge

import (
	"errors"

	grpc1 "github.com/gogo/protobuf/grpc"
	zlog "github.com/rs/zerolog/log"
	"gitlab.com/oppy-finance/oppy-bridge/pubchain"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
)

// FeedTx feed the tx with the given
func (oc *OppyChainInstance) FeedTx(conn grpc1.ClientConn, lastPoolInfo *vaulttypes.PoolInfo, pi *pubchain.Instance) error {
	// we always increase the account seq regardless the tx successful or not
	currentPool := lastPoolInfo.CreatePool.PoolAddr
	acc, err := queryAccount(conn, currentPool.String(), "")
	if err != nil {
		oc.logger.Error().Err(err).Msgf("fail to query the account")
		return errors.New("invalid account query")
	}

	inboundItems := pi.PopItem(pubchain.GroupSign)
	if inboundItems == nil {
		oc.logger.Info().Msgf("empty queue")
		return nil
	}

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

		el.SetAccountInfo(accNum, accSeq, address, poolPubkey)
		accSeq++
		pi.InboundReqChan <- el
	}
	return nil
}
