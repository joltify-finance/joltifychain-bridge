package pubchain

import (
	"context"

	"gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
)

func (pi *Instance) FeedTx(currentBlockHeight int64, lastPoolInfo *vaulttypes.PoolInfo, outboundReqs []*common.OutBoundReq) error {
	// we always increase the account seq regardless the tx successful or not
	poolEthAddress, err := misc.PoolPubKeyToEthAddress(lastPoolInfo.CreatePool.GetPoolPubKey())
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to convert the poolpubkey to eth address")
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	nonce, err := pi.EthClient.PendingNonceAt(ctx, poolEthAddress)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to get the nonce of the given pool address")
		return err
	}

	roundBlockHeight := currentBlockHeight / ROUNDBLOCK
	// for BSC we need to use the next nonce while for oppy, we used the returned nonce
	for _, el := range outboundReqs {
		el.SetItemHeightAndNonce(roundBlockHeight, currentBlockHeight, nonce)
		nonce += 1
	}
	return nil
}
