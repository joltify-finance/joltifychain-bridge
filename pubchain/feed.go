package pubchain

import (
	"context"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

func (pi *PubChainInstance) FeedTx(currentBlockHeight int64, lastPoolInfo *vaulttypes.PoolInfo, outboundReqs []*common.OutBoundReq) error {

	//we always increase the account seq regardless the tx successful or not

	poolEthAddress, err := misc.PoolPubKeyToEthAddress(lastPoolInfo.CreatePool.GetPoolPubKey())
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	nonce, err := pi.EthClient.PendingNonceAt(ctx, poolEthAddress)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to get the nonce of the given pool address")
		return err
	}

	roundBlockHeight := currentBlockHeight / ROUNDBLOCK

	for _, el := range outboundReqs {
		el.SetItemHeightandNonce(roundBlockHeight, nonce)
		nonce += 1
	}
	return nil

}
