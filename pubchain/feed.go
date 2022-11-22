package pubchain

import (
	"context"

	"gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
)

func (pi *Instance) FeedTx(lastPoolInfo *vaulttypes.PoolInfo, outboundReqs []*common.OutBoundReq, chainType string) error {
	// we always increase the account seq regardless the tx successful or not
	client := pi.GetChainClient(chainType)
	poolEthAddress, err := misc.PoolPubKeyToEthAddress(lastPoolInfo.CreatePool.GetPoolPubKey())
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to convert the poolpubkey to eth address")
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	nonce, err := client.getPendingNonceWithLock(ctx, poolEthAddress)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to get the nonce of the given pool address")
		return err
	}

	// for BSC we need to use the next nonce while for oppy, we used the returned nonce
	for _, el := range outboundReqs {
		el.SetItemNonce(poolEthAddress, nonce)
		nonce++
	}
	return nil
}
