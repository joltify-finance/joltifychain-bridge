package cosbridge

import (
	"errors"

	"gitlab.com/joltify/joltifychain-bridge/common"

	grpc1 "github.com/gogo/protobuf/grpc"
	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	zlog "github.com/rs/zerolog/log"
	"gitlab.com/joltify/joltifychain-bridge/pubchain"
)

// FeedTx feed the tx with the given
func (jc *JoltChainInstance) FeedTx(conn grpc1.ClientConn, lastPoolInfo *vaulttypes.PoolInfo, pi *pubchain.Instance) error {
	// we always increase the Account seq regardless the tx successful or not

	found, err := jc.CheckWhetherSigner(lastPoolInfo)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to check whether we are the node submit the mint request")
		return err
	}
	if !found {
		zlog.Info().Msgf("we are not the signer")
		return nil
	}

	currentPool := lastPoolInfo.CreatePool.PoolAddr
	acc, err := common.QueryAccount(conn, currentPool.String(), "")
	if err != nil {
		jc.logger.Error().Err(err).Msgf("fail to query the Account")
		return errors.New("invalid Account query")
	}

	inboundItems := pi.PopItem(pubchain.GroupSign)
	if inboundItems == nil {
		jc.logger.Info().Msgf("empty queue")
		return nil
	}

	accNum := acc.GetAccountNumber()
	accSeq := acc.GetSequence()
	address := acc.GetAddress()
	poolPubkey := lastPoolInfo.CreatePool.PoolPubKey
	for _, el := range inboundItems {
		el.SetAccountInfo(accNum, accSeq, address, poolPubkey)
		accSeq++
	}
	pi.InboundReqChan <- inboundItems
	return nil
}
