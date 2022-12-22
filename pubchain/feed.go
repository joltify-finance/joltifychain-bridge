package pubchain

import (
	"context"

	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
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
