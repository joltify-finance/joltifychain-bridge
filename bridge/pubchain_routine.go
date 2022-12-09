package bridge

import (
	"math/big"

	"github.com/ethereum/go-ethereum/ethclient"
	zlog "github.com/rs/zerolog/log"
	"gitlab.com/oppy-finance/oppy-bridge/cosbridge"
	"gitlab.com/oppy-finance/oppy-bridge/monitor"
	"gitlab.com/oppy-finance/oppy-bridge/pubchain"
	"go.uber.org/atomic"
)

func pubchainProcess(pi *pubchain.Instance, oppyChain *cosbridge.OppyChainInstance, oppyGrpc string, metric *monitor.Metric, blockHead *pubchain.BlockHead, pubRollbackGap int64, failedOutbound *atomic.Int32, outboundPauseHeight *uint64, outBoundWait *atomic.Bool, outBoundProcessDone, inKeygenInProgress *atomic.Bool, firstTimeOutbound *bool, previousTssBlockOutBound *int64) {
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

	_, err = oppyChain.GetLastBlockHeightWithLock()
	if err != nil {
		zlog.Error().Err(err).Msgf("we have reset the oppychain grpc as it is faild to be connected")
		return
	}

	updateHealthCheck(pi, metric)

	// process block with rollback gap
	processableBlockHeight := big.NewInt(0).Sub(head.Number, big.NewInt(pubRollbackGap))

	pools := oppyChain.GetPool()
	if len(pools) < 2 || pools[1] == nil {
		// this is need once we resume the bridge to avoid the panic that the pool address has not been filled
		zlog.Logger.Warn().Msgf("we do not have 2 pools to start the tx")
		return
	}

	amISigner, err := oppyChain.CheckWhetherSigner(pools[1].PoolInfo)
	if err != nil {
		zlog.Logger.Error().Err(err).Msg("fail to check whether we are the node submit the mint request")
		return
	}

	if !amISigner {
		zlog.Logger.Info().Msg("we are not the signer, we quite the block process")
		return
	}

	err = pi.ProcessNewBlock(blockHead.ChainType, chainInfo, processableBlockHeight)
	pi.CurrentHeight = head.Number.Int64()
	if err != nil {
		zlog.Logger.Error().Err(err).Msg("fail to process the inbound block")
	}
	isMoveFund := false
	previousPool, height := pi.PopMoveFundItemAfterBlock(head.Number.Int64())

	if previousPool != nil {
		// we move fund in the public chain
		ethClient, err := ethclient.Dial(chainInfo.WsAddr)
		if err != nil {
			pi.AddMoveFundItem(previousPool, height)
			zlog.Logger.Error().Err(err).Msg("fail to dial the websocket")
		}
		if ethClient != nil {
			isMoveFund = pi.MoveFound(height, chainInfo, previousPool, ethClient)
			ethClient.Close()
		}
	}
	if isMoveFund {
		// once we move fund, we do not send tx to be processed
		return
	}

	if failedOutbound.Load() > 5 {
		mid := (latestHeight.NumberU64() / uint64(ROUNDBLOCK)) + 1
		*outboundPauseHeight = mid * uint64(ROUNDBLOCK)
		failedOutbound.Store(0)
		outBoundWait.Store(true)
	}

	if latestHeight.NumberU64() < *outboundPauseHeight {
		zlog.Logger.Warn().Msgf("to many errors for outbound we wait for %v blocks to continue", *outboundPauseHeight-latestHeight.NumberU64())
		if latestHeight.NumberU64() == *outboundPauseHeight-1 {
			zlog.Info().Msgf("we now load the onhold tx")
			putOnHoldBlockOutBoundBack(oppyGrpc, chainInfo, oppyChain)
		}
		return
	}

	outBoundWait.Store(false)

	if !outBoundProcessDone.Load() {
		zlog.Warn().Msgf("the previous outbound has not been fully processed, we do not feed more tx")
		metric.UpdateOutboundTxNum(float64(oppyChain.Size()))
		return
	}

	if inKeygenInProgress.Load() {
		zlog.Warn().Msgf("we are in keygen process, we do not feed more tx")
		metric.UpdateOutboundTxNum(float64(oppyChain.Size()))
		return
	}

	if oppyChain.IsEmpty() {
		zlog.Logger.Debug().Msgf("the inbound queue is empty, we put all onhold back")
		putOnHoldBlockOutBoundBack(oppyGrpc, chainInfo, oppyChain)
	}

	// todo we need also to add the check to avoid send tx near the churn blocks
	if processableBlockHeight.Int64()-*previousTssBlockOutBound >= cosbridge.GroupBlockGap && !oppyChain.IsEmpty() {
		// if we do not have enough tx to process, we wait for another round
		if oppyChain.Size() < pubchain.GroupSign && *firstTimeOutbound {
			*firstTimeOutbound = false
			metric.UpdateOutboundTxNum(float64(oppyChain.Size()))
			return
		}

		zlog.Logger.Warn().Msgf("we feed the outbound tx now %v", pools[1].PoolInfo.CreatePool.PoolAddr.String())

		outboundItems := oppyChain.PopItem(pubchain.GroupSign, blockHead.ChainType)

		if outboundItems == nil {
			zlog.Logger.Info().Msgf("empty queue")
			return
		}

		err = pi.FeedTx(pools[1].PoolInfo, outboundItems, blockHead.ChainType)
		if err != nil {
			zlog.Logger.Error().Err(err).Msgf("fail to feed the tx")
			return
		}
		*previousTssBlockOutBound = processableBlockHeight.Int64()
		*firstTimeOutbound = true
		metric.UpdateOutboundTxNum(float64(oppyChain.Size()))
		oppyChain.OutboundReqChan <- outboundItems
		outBoundProcessDone.Store(false)
	}
}
