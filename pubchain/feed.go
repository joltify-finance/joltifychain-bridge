package pubchain

import (
	"context"

	"github.com/cosmos/cosmos-sdk/types"
	zlog "github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

func (pi *Instance) FeedTx(lastPoolInfo *vaulttypes.PoolInfo, outboundReqs []*common.OutBoundReq, chainType string) error {
	// we always increase the account seq regardless the tx successful or not
	client := pi.GetChainClientERC20(chainType)
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

	// for BSC we need to use the next nonce while for joltify, we used the returned nonce
	for _, el := range outboundReqs {
		el.SetItemNonce(poolEthAddress.Bytes(), nonce, lastPoolInfo.CreatePool.PoolPubKey, 0)
		nonce++
	}
	return nil
}

func (pi *Instance) FeedTxCosmos(lastPoolInfo *vaulttypes.PoolInfo, outboundReqs []*common.OutBoundReq) error {
	// we always increase the account seq regardless the tx successful or not

	grpcClient, err := grpc.Dial(pi.CosChain.CosHandler.GrpcAddr, grpc.WithInsecure())
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to dial the cosmos grpc")
		return err
	}
	defer grpcClient.Close()

	poolAddress, err := misc.PoolPubKeyToJoltifyAddress(lastPoolInfo.CreatePool.GetPoolPubKey())
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to convert the poolpubkey to eth address")
		return err
	}

	atomAddress := types.MustBech32ifyAddressBytes("cosmos", poolAddress)
	acc, err := common.QueryAccount(grpcClient, atomAddress, "")
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to query the account sequence")
		return err
	}

	nonce := acc.GetSequence()
	accNum := acc.GetAccountNumber()
	// for BSC we need to use the next nonce while for joltify, we used the returned nonce
	for _, el := range outboundReqs {
		el.SetItemNonce(poolAddress.Bytes(), nonce, lastPoolInfo.CreatePool.PoolPubKey, accNum)
		nonce++
	}
	return nil
}
