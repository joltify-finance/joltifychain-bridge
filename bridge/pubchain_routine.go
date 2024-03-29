package bridge

import (
	"math/big"

	"github.com/ethereum/go-ethereum/ethclient"
	zlog "github.com/rs/zerolog/log"
	"gitlab.com/joltify/joltifychain-bridge/cosbridge"
	"gitlab.com/joltify/joltifychain-bridge/monitor"
	"gitlab.com/joltify/joltifychain-bridge/pubchain"
	"go.uber.org/atomic"
)

func pubchainProcess(pi *pubchain.Instance, joltChain *cosbridge.JoltChainInstance, oppyGrpc string, metric *monitor.Metric, blockHead *pubchain.BlockHead, pubRollbackGap int64, failedOutbound *atomic.Int32, outboundPauseHeight *OutboundPauseHeight, outBoundWait *atomic.Bool, outBoundProcessDone, inKeygenInProgress *atomic.Bool, firstTimeOutbound *bool, previousTssBlockOutBound *PreviousTssBlockOutBound) {
	head := blockHead.Head
	chainInfo := pi.GetChainClient(blockHead.ChainType)
	if chainInfo == nil {
		return
	}
	latestHeight, err := chainInfo.GetBlockByNumberWithLock(nil)
	if err != nil {
		zlog.Error().Err(err).Msgf("fail to get the latest public block")
		return
	}

	joltBlockHeight, err := joltChain.GetLastBlockHeightWithLock()
	if err != nil {
		zlog.Error().Err(err).Msgf("we have reset the oppychain grpc as it is faild to be connected")
		return
	}

	updateHealthCheck(pi, metric)

	// process block with rollback gap
	processableBlockHeight := big.NewInt(0).Sub(head.Number, big.NewInt(pubRollbackGap))

	pools := joltChain.GetPool()
	if len(pools) < 2 || pools[1] == nil {
		// this is need once we resume the bridge to avoid the panic that the pool address has not been filled
		zlog.Logger.Warn().Msgf("we do not have 2 pools to start the tx")
		return
	}

	amISigner, err := joltChain.CheckWhetherSigner(pools[1].PoolInfo)
	if err != nil {
		zlog.Logger.Error().Err(err).Msg("fail to check whether we are the node submit the mint request")
		return
	}

	if !amISigner {
		zlog.Logger.Info().Msg("we are not the signer, we quite the block process")
		return
	}

	err = pi.ProcessNewBlock(blockHead.ChainType, chainInfo, processableBlockHeight, joltChain.FeeModule, oppyGrpc)
	if err != nil {
		zlog.Logger.Error().Err(err).Msg("fail to process the inbound block")
	}
	isMoveFund := false
	previousMoveFundItem, height := pi.PopMoveFundItemAfterBlock(joltBlockHeight, chainInfo.ChainType)

	if previousMoveFundItem != nil {
		// we move fund in the public chain
		ethClient, err := ethclient.Dial(chainInfo.WsAddr)
		if err != nil {
			pi.AddMoveFundItem(previousMoveFundItem.PoolInfo, height, chainInfo.ChainType)
			zlog.Logger.Error().Err(err).Msg("fail to dial the websocket")
		}
		if ethClient != nil {
			isMoveFund = pi.MoveFound(height, chainInfo, previousMoveFundItem.PoolInfo, ethClient)
			ethClient.Close()
		}
	}
	if isMoveFund {
		// once we move fund, we do not send tx to be processed
		return
	}

	if failedOutbound.Load() > 5 {
		mid := (latestHeight.NumberU64() / uint64(ROUNDBLOCK)) + 1
		outboundPauseHeight.SetHeight(mid*uint64(ROUNDBLOCK), chainInfo.ChainType)
		failedOutbound.Store(0)
		outBoundWait.Store(true)
	}

	if latestHeight.NumberU64() < outboundPauseHeight.GetHeight(chainInfo.ChainType) {
		zlog.Logger.Warn().Msgf("to many errors for outbound we wait for %v blocks to continue", outboundPauseHeight.GetHeight(chainInfo.ChainType)-latestHeight.NumberU64())
		if latestHeight.NumberU64() == outboundPauseHeight.GetHeight(chainInfo.ChainType)-1 {
			zlog.Info().Msgf("we now load the onhold tx")
			putOnHoldBlockOutBoundBack(oppyGrpc, chainInfo, joltChain)
		}
		return
	}

	outBoundWait.Store(false)

	if !outBoundProcessDone.Load() {
		zlog.Warn().Msgf("the previous outbound has not been fully processed, we do not feed more tx")
		metric.UpdateOutboundTxNum(float64(joltChain.Size()))
		return
	}

	if inKeygenInProgress.Load() {
		zlog.Warn().Msgf("we are in keygen process, we do not feed more tx")
		metric.UpdateOutboundTxNum(float64(joltChain.Size()))
		return
	}

	if joltChain.IsEmpty() {
		zlog.Logger.Debug().Msgf("the inbound queue is empty, we put all onhold back")
		putOnHoldBlockOutBoundBack(oppyGrpc, chainInfo, joltChain)
	}

	// todo we need also to add the check to avoid send tx near the churn blocks
	if processableBlockHeight.Int64()-previousTssBlockOutBound.GetHeight(blockHead.ChainType) >= cosbridge.GroupBlockGap && !joltChain.IsEmpty() {
		// if we do not have enough tx to process, we wait for another round
		if joltChain.Size() < pubchain.GroupSign && *firstTimeOutbound {
			*firstTimeOutbound = false
			metric.UpdateOutboundTxNum(float64(joltChain.Size()))
			return
		}

		zlog.Logger.Warn().Msgf("we feed the outbound tx now %v (processableBlockHeight:%v, previousTssBlockOutBound:%v)", pools[1].PoolInfo.CreatePool.PoolAddr.String(), processableBlockHeight, previousTssBlockOutBound.GetHeight(blockHead.ChainType))

		outboundItems := joltChain.PopItem(pubchain.GroupSign, blockHead.ChainType)

		if outboundItems == nil {
			zlog.Logger.Info().Msgf("empty queue for chain %v", blockHead.ChainType)
			return
		}

		err = pi.FeedTx(pools[1].PoolInfo, outboundItems, blockHead.ChainType)
		if err != nil {
			zlog.Logger.Error().Err(err).Msgf("fail to feed the tx")
			return
		}
		previousTssBlockOutBound.SetHeight(processableBlockHeight.Int64(), blockHead.ChainType)
		*firstTimeOutbound = true
		metric.UpdateOutboundTxNum(float64(joltChain.Size()))
		joltChain.OutboundReqChan <- outboundItems
		outBoundProcessDone.Store(false)
	}
}
