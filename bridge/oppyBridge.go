package bridge

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"os/signal"
	"path"
	"strconv"
	"sync"
	"syscall"
	"time"

	testypes "github.com/tendermint/tendermint/types" //nolint:gofumpt,golint,typecheck

	"github.com/cenkalti/backoff"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"gitlab.com/oppy-finance/oppy-bridge/storage"
	"gitlab.com/oppy-finance/oppy-bridge/tokenlist"
	"go.uber.org/atomic"
	"golang.org/x/term"
	"google.golang.org/grpc"

	"github.com/ethereum/go-ethereum/common"

	common2 "gitlab.com/oppy-finance/oppy-bridge/common"

	"gitlab.com/oppy-finance/oppy-bridge/monitor"
	"gitlab.com/oppy-finance/oppy-bridge/tssclient"

	zlog "github.com/rs/zerolog/log"
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/oppybridge"
	"gitlab.com/oppy-finance/oppy-bridge/pubchain"
)

// ROUNDBLOCK we may need to increase it as we increase the time for keygen/keysign and join party
var (
	ROUNDBLOCK = 100
)

func readPassword() ([]byte, error) {
	var fd int
	fmt.Printf("please input the password:")
	if term.IsTerminal(syscall.Stdin) {
		fd = syscall.Stdin
	} else {
		tty, err := os.Open("/dev/tty")
		if err != nil {
			return nil, errors.Wrap(err, "error allocating terminal")
		}
		defer tty.Close()
		fd = int(tty.Fd())
	}

	pass, err := term.ReadPassword(fd)
	return pass, err
}

// NewBridgeService starts the new bridge service
func NewBridgeService(config config.Config) {
	pass, err := readPassword()
	if err != nil {
		log.Fatalf("fail to read the password with err %v\n", err)
	}
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	passcodeLength := 32
	passcode := make([]byte, passcodeLength)
	n, err := os.Stdin.Read(passcode)
	if err != nil {
		cancel()
		return
	}
	if n > passcodeLength {
		log.Fatalln("the passcode is too long")
		return
	}

	metrics := monitor.NewMetric()
	if config.EnableMonitor {
		metrics.Enable()
	}

	// now we load the token list

	tokenPath := path.Join(config.HomeDir, config.TokenListPath)
	tl, err := tokenlist.NewTokenList(tokenPath, int64(config.TokenListUpdateGap))
	if err != nil {
		fmt.Printf("fail to load token list")
		cancel()
		return
	}

	// fixme, in docker it needs to be changed to basehome
	tssServer, _, err := tssclient.StartTssServer(config.HomeDir, config.TssConfig)
	if err != nil {
		log.Fatalln("fail to start the tss")
		return
	}

	oppyBridge, err := oppybridge.NewOppyBridge(config.OppyChain.GrpcAddress, config.OppyChain.WsAddress, tssServer, tl)
	if err != nil {
		log.Fatalln("fail to create the invoice oppy_bridge", err)
		return
	}

	keyringPath := path.Join(config.HomeDir, config.KeyringAddress)

	dat, err := ioutil.ReadFile(keyringPath)
	if err != nil {
		log.Fatalln("error in read keyring file")
		return
	}

	err = oppyBridge.Keyring.ImportPrivKey("operator", string(dat), string(pass))
	if err != nil {
		cancel()
		return
	}
	pass = []byte{}
	_ = pass

	defer func() {
		err := oppyBridge.TerminateBridge()
		if err != nil {
			return
		}
	}()

	err = oppyBridge.InitValidators(config.OppyChain.HTTPAddress)
	if err != nil {
		fmt.Printf("error in init the validators %v", err)
		cancel()
		return
	}
	tssHTTPServer := NewOppyHttpServer(ctx, config.TssConfig.HTTPAddr, oppyBridge.GetTssNodeID(), oppyBridge)

	wg.Add(1)
	ret := tssHTTPServer.Start(&wg)
	if ret != nil {
		cancel()
		return
	}

	// now we monitor the bsc transfer event
	pubChainInstance, err := pubchain.NewChainInstance(config.PubChainConfig.WsAddress, tssServer, tl, &wg)
	if err != nil {
		fmt.Printf("fail to connect the public pub_chain with address %v\n", config.PubChainConfig.WsAddress)
		cancel()
		return
	}

	fsm := storage.NewTxStateMgr(config.HomeDir)
	// now we load the existing outbound requests
	items, err := fsm.LoadOutBoundState()
	if err != nil {
		fmt.Printf("we do not need to have the items to be loaded")
	}
	if items != nil {
		for _, el := range items {
			oppyBridge.AddItem(el)
		}
		fmt.Printf("we have loaded the unprocessed outbound tx\n")
	}

	// now we load the existing inbound requests
	itemsIn, err := fsm.LoadInBoundState()
	if err != nil {
		fmt.Printf("we do not need to have the items to be loaded")
	}
	if itemsIn != nil {
		for _, el := range itemsIn {
			pubChainInstance.AddItem(el)
		}
		fmt.Printf("we have loaded the unprocessed inbound tx\n")
	}

	// now we load the pending outboundtx
	pendingManager := storage.NewPendingTxStateMgr(config.HomeDir)
	pendingItems, err := pendingManager.LoadPendingItems()
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to load the pending items!!")
	}

	if len(pendingItems) != 0 {
		oppyBridge.Import(pendingItems)
	}

	moveFundMgr := storage.NewMoveFundStateMgr(config.HomeDir)

	pubItems, err := moveFundMgr.LoadPendingItems()
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to load the pending move fund, we skip!!")
	}

	if len(pubItems) != 0 {
		for _, item := range pubItems {
			pubChainInstance.AddMoveFundItem(item, item.Height)
		}
	}

	addEventLoop(ctx, &wg, oppyBridge, pubChainInstance, metrics, int64(config.OppyChain.RollbackGap), int64(config.PubChainConfig.RollbackGap), tl, config.PubChainConfig.WsAddress, config.OppyChain.GrpcAddress)
	<-c
	cancel()
	wg.Wait()

	items1 := oppyBridge.DumpQueue()
	for _, el := range items1 {
		oppyBridge.AddItem(el)
	}

	items2 := pubChainInstance.DumpQueue()
	for _, el := range items2 {
		pubChainInstance.AddItem(el)
	}

	itemsexported := oppyBridge.ExportItems()
	err = fsm.SaveOutBoundState(itemsexported)
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to save the outbound requests!!!")
	}

	itemsInexported := pubChainInstance.ExportItems()
	err = fsm.SaveInBoundState(itemsInexported)
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to save the outbound requests!!!")
	}

	exportedPending := oppyBridge.Export()

	err = pendingManager.SavePendingItems(exportedPending)
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to save the pending requests!!!")
	}

	// we now save the move fund items
	pubItemsSave := pubChainInstance.ExportMoveFundItems()

	err = moveFundMgr.SavePendingItems(pubItemsSave)
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to save move fund items")
	}

	zlog.Logger.Info().Msgf("we quit the bridge gracefully")
}

func addEventLoop(ctx context.Context, wg *sync.WaitGroup, oppyChain *oppybridge.OppyChainInstance, pi *pubchain.Instance, metric *monitor.Metric, oppyRollbackGap int64, pubRollbackGap int64, tl *tokenlist.TokenList, pubChainWsAddress, oppyGrpc string) {
	ctxLocal, cancelLocal := context.WithTimeout(context.Background(), time.Second*5)
	defer cancelLocal()

	err := oppyChain.AddSubscribe(ctxLocal)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return
	}

	// pubNewBlockChan is the channel for the new blocks for the public chain
	subscriptionCtx, cancelsubscription := context.WithCancel(context.Background())
	err = pi.StartSubscription(subscriptionCtx, wg)
	if err != nil {
		fmt.Printf("fail to subscribe the token transfer with err %v\n", err)
		cancelsubscription()
		return
	}

	blockHeight, err := oppyChain.GetLastBlockHeightWithLock()
	if err != nil {
		fmt.Printf("we fail to get the latest block height")
		cancelsubscription()
		return
	}

	previousTssBlockInbound := blockHeight
	previousTssBlockOutBound := blockHeight
	firstTimeInbound := true
	firstTimeOutbound := true
	localSubmitLocker := sync.Mutex{}

	failedInbound := atomic.NewInt32(0)
	inboundPauseHeight := int64(0)
	outboundPauseHeight := uint64(0)
	failedOutbound := atomic.NewInt32(0)
	inBoundWait := atomic.NewBool(false)
	outBoundWait := atomic.NewBool(false)

	inBoundProcessDone := atomic.NewBool(true)
	outBoundProcessDone := atomic.NewBool(true)
	inKeygenInProgress := atomic.NewBool(false)

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				cancelsubscription()
				zlog.Info().Msgf("we quit the whole process")
				return
				// process the update of the validators

			case vals := <-oppyChain.CurrentNewValidator:
				oppyChain.ChannelQueueValidator <- vals

			case vals := <-oppyChain.ChannelQueueValidator:

				height, err := oppyChain.GetLastBlockHeightWithLock()
				if err != nil {
					continue
				}

				_, ok := vals.Data.(testypes.EventDataNewBlock)
				if !ok {
					continue
				}

				err = oppyChain.HandleUpdateValidators(height, wg, inKeygenInProgress, false)
				if err != nil {
					zlog.Logger.Info().Msgf("error in handle update validator")
					continue
				}

			// process the new oppy block, validator may need to submit the pool address
			case block := <-oppyChain.CurrentNewBlockChan:
				oppyChain.ChannelQueueNewBlock <- block

			case block := <-oppyChain.ChannelQueueNewBlock:
				wg.Add(1)
				go func() {
					defer wg.Done()
					pi.HealthCheckAndReset()
				}()

				// we add the pubChain healthCheck
				grpcClient, err := grpc.Dial(oppyGrpc, grpc.WithInsecure())
				if err != nil {
					zlog.Logger.Error().Err(err).Msgf("fail to dial at queue new block")
					continue
				}

				latestHeight, err := oppybridge.GetLastBlockHeight(grpcClient)
				if err != nil {
					zlog.Logger.Error().Err(err).Msgf("fail to get the latest block height")
					grpcClient.Close()
					continue
				}

				currentProcessBlockHeight := block.Data.(testypes.EventDataNewBlock).Block.Height

				ok, _ := oppyChain.CheckAndUpdatePool(grpcClient, latestHeight)
				if !ok {
					// it is okay to fail to submit a pool address as other nodes can submit, as long as 2/3 nodes submit, it is fine.
					zlog.Logger.Warn().Msgf("we fail to submit the new pool address")
				}

				// now we check whether we need to update the pool
				// we query the pool from the chain directly.
				poolInfo, err := oppyChain.QueryLastPoolAddress(grpcClient)
				if err != nil {
					zlog.Logger.Error().Err(err).Msgf("error in get pool with error %v", err)
					grpcClient.Close()
					continue
				}
				if len(poolInfo) != 2 {
					zlog.Logger.Warn().Msgf("the pool only have %v address, bridge will not work", len(poolInfo))
					grpcClient.Close()
					continue
				}

				currentPool := pi.GetPool()

				pools := oppyChain.GetPool()

				// this means the pools has not been filled with two address
				if currentPool[0] == nil {
					for _, el := range poolInfo {
						err := pi.UpdatePool(el)
						if err != nil {
							zlog.Log().Err(err).Msgf("fail to update the pool")
						}
						oppyChain.UpdatePool(el)
					}
					grpcClient.Close()
					continue
				}

				if NeedUpdate(poolInfo, currentPool) {
					err := pi.UpdatePool(poolInfo[0])
					if err != nil {
						zlog.Log().Err(err).Msgf("fail to update the pool")
					}
					previousPool := oppyChain.UpdatePool(poolInfo[0])
					if previousPool.Pk != poolInfo[0].CreatePool.PoolPubKey {
						// we force the first try of the tx to be run without blocking by the block wait
						pi.AddMoveFundItem(previousPool, latestHeight-config.MINCHECKBLOCKGAP+5)
					}
				}

				// we update the tx new, if there exits a processable block
				processableBlockHeight := int64(0)
				if currentProcessBlockHeight > oppyRollbackGap {
					processableBlockHeight = currentProcessBlockHeight - oppyRollbackGap
					processableBlock, err := oppyChain.GetBlockByHeight(grpcClient, processableBlockHeight)
					if err != nil {
						zlog.Logger.Error().Err(err).Msgf("error in get block to process %v", err)
						grpcClient.Close()
						continue
					}
					oppyChain.DeleteExpired(currentProcessBlockHeight)

					amISigner, err := oppyChain.CheckWhetherSigner(pools[1].PoolInfo)
					if err != nil {
						zlog.Logger.Error().Err(err).Msg("fail to check whether we are the node submit the mint request")
						grpcClient.Close()
						continue
					}
					if !amISigner {
						zlog.Logger.Info().Msgf("we are not the signer in oppy chain check")
						grpcClient.Close()
						continue
					}

					if amISigner {
						// here we process the outbound tx
						for _, el := range processableBlock.Data.Txs {
							oppyChain.CheckOutBoundTx(grpcClient, processableBlockHeight, el)
						}
					}
				}

				oppyChain.CurrentHeight = currentProcessBlockHeight

				// we update the token list, if the current block height refresh the update mark
				err = tl.UpdateTokenList(oppyChain.CurrentHeight)
				if err != nil {
					zlog.Logger.Warn().Msgf("error in updating token list %v", err)
				}

				if currentProcessBlockHeight%pubchain.PRICEUPDATEGAP == 0 {
					fee, _, _, _, err := pi.GetFeeLimitWithLock()
					if err == nil {
						oppyChain.UpdatePubChainFee(fee.Int64())
					} else {
						zlog.Logger.Error().Err(err).Msg("fail to get the suggest gas price")
					}
				}

				if failedInbound.Load() > 5 {
					failedInbound.Store(0)
					mid := (latestHeight / int64(ROUNDBLOCK)) + 1
					inboundPauseHeight = mid * int64(ROUNDBLOCK)
					inBoundWait.Store(true)
				}

				if latestHeight < inboundPauseHeight {
					zlog.Logger.Warn().Msgf("to many errors for inbound, we wait for %v blocks to continue", inboundPauseHeight-latestHeight)
					if latestHeight == inboundPauseHeight-1 {
						zlog.Info().Msgf("we now load the onhold tx")
						putOnHoldBlockInBoundBack(oppyGrpc, pi, oppyChain)
					}
					grpcClient.Close()

					continue
				}
				inBoundWait.Store(false)

				if pi.IsEmpty() {
					zlog.Logger.Debug().Msgf("the inbound queue is empty, we put all onhold back")
					putOnHoldBlockInBoundBack(oppyGrpc, pi, oppyChain)
				}

				// todo we need also to add the check to avoid send tx near the churn blocks
				if processableBlockHeight-previousTssBlockInbound >= pubchain.GroupBlockGap && !pi.IsEmpty() {
					// if we do not have enough tx to process, we wait for another round
					if pi.Size() < pubchain.GroupSign && firstTimeInbound {
						firstTimeInbound = false
						metric.UpdateInboundTxNum(float64(pi.Size()))
						grpcClient.Close()
						continue
					}

					if !inBoundProcessDone.Load() {
						zlog.Warn().Msgf("the previous inbound has not been fully processed, we do not feed more tx")
						grpcClient.Close()
						continue
					}
					if inKeygenInProgress.Load() {
						zlog.Warn().Msgf("we are in keygen process, we do not feed more tx")
						metric.UpdateOutboundTxNum(float64(oppyChain.Size()))
						continue
					}

					zlog.Logger.Warn().Msgf("we feed the inbound tx now %v", pools[1].PoolInfo.CreatePool.String())

					err = oppyChain.FeedTx(grpcClient, pools[1].PoolInfo, pi)
					if err != nil {
						zlog.Logger.Error().Err(err).Msgf("fail to feed the tx")
					}

					inBoundProcessDone.Store(false)
					previousTssBlockInbound = currentProcessBlockHeight
					firstTimeInbound = true
					metric.UpdateInboundTxNum(float64(pi.Size()))
				}

				grpcClient.Close()

			case head := <-pi.SubChannelNow:
				pi.ChannelQueue <- head
				// process the public chain new block event
			case head := <-pi.ChannelQueue:

				latestHeight, err := pi.GetBlockByNumberWithLock(nil)
				if err != nil {
					zlog.Error().Err(err).Msgf("fail to get the latest public block")
					continue
				}

				_, err = oppyChain.GetLastBlockHeightWithLock()
				if err != nil {
					zlog.Error().Err(err).Msgf("we have reset the oppychain grpc as it is faild to be connected")
					continue
				}

				updateHealthCheck(pi, metric)

				// process block with rollback gap
				processableBlockHeight := big.NewInt(0).Sub(head.Number, big.NewInt(pubRollbackGap))

				pools := oppyChain.GetPool()
				if len(pools) < 2 || pools[1] == nil {
					// this is need once we resume the bridge to avoid the panic that the pool address has not been filled
					zlog.Logger.Warn().Msgf("we do not have 2 pools to start the tx")
					continue
				}

				amISigner, err := oppyChain.CheckWhetherSigner(pools[1].PoolInfo)
				if err != nil {
					zlog.Logger.Error().Err(err).Msg("fail to check whether we are the node submit the mint request")
					continue
				}

				if !amISigner {
					zlog.Logger.Info().Msg("we are not the signer, we quite the block process")
					continue
				}

				err = pi.ProcessNewBlock(processableBlockHeight)
				pi.CurrentHeight = head.Number.Int64()
				if err != nil {
					zlog.Logger.Error().Err(err).Msg("fail to process the inbound block")
				}
				isMoveFund := false
				previousPool, height := pi.PopMoveFundItemAfterBlock(head.Number.Int64())

				if previousPool != nil {
					// we move fund in the public chain
					ethClient, err := ethclient.Dial(pubChainWsAddress)
					if err != nil {
						pi.AddMoveFundItem(previousPool, height)
						zlog.Logger.Error().Err(err).Msg("fail to dial the websocket")
					}
					if ethClient != nil {
						isMoveFund = pi.MoveFound(height, pubChainWsAddress, previousPool, ethClient)
						ethClient.Close()
					}
				}
				if isMoveFund {
					// once we move fund, we do not send tx to be processed
					continue
				}

				if failedOutbound.Load() > 5 {
					mid := (latestHeight.NumberU64() / uint64(ROUNDBLOCK)) + 1
					outboundPauseHeight = mid * uint64(ROUNDBLOCK)
					failedOutbound.Store(0)
					outBoundWait.Store(true)
				}

				if latestHeight.NumberU64() < outboundPauseHeight {
					zlog.Logger.Warn().Msgf("to many errors for outbound we wait for %v blocks to continue", outboundPauseHeight-latestHeight.NumberU64())
					if latestHeight.NumberU64() == outboundPauseHeight-1 {
						zlog.Info().Msgf("we now load the onhold tx")
						putOnHoldBlockOutBoundBack(oppyGrpc, pi, oppyChain)
					}
					continue
				}

				outBoundWait.Store(false)

				if !outBoundProcessDone.Load() {
					zlog.Warn().Msgf("the previous outbound has not been fully processed, we do not feed more tx")
					metric.UpdateOutboundTxNum(float64(oppyChain.Size()))
					continue
				}

				if inKeygenInProgress.Load() {
					zlog.Warn().Msgf("we are in keygen process, we do not feed more tx")
					metric.UpdateOutboundTxNum(float64(oppyChain.Size()))
					continue
				}

				if oppyChain.IsEmpty() {
					zlog.Logger.Debug().Msgf("the inbound queue is empty, we put all onhold back")
					putOnHoldBlockOutBoundBack(oppyGrpc, pi, oppyChain)
				}

				// todo we need also to add the check to avoid send tx near the churn blocks
				if processableBlockHeight.Int64()-previousTssBlockOutBound >= oppybridge.GroupBlockGap && !oppyChain.IsEmpty() {
					// if we do not have enough tx to process, we wait for another round
					if oppyChain.Size() < pubchain.GroupSign && firstTimeOutbound {
						firstTimeOutbound = false
						metric.UpdateOutboundTxNum(float64(oppyChain.Size()))
						continue
					}

					zlog.Logger.Warn().Msgf("we feed the outbound tx now %v", pools[1].PoolInfo.CreatePool.String())

					outboundItems := oppyChain.PopItem(pubchain.GroupSign)

					if outboundItems == nil {
						zlog.Logger.Info().Msgf("empty queue")
						continue
					}

					err = pi.FeedTx(pools[1].PoolInfo, outboundItems)
					if err != nil {
						zlog.Logger.Error().Err(err).Msgf("fail to feed the tx")
						continue
					}
					previousTssBlockOutBound = processableBlockHeight.Int64()
					firstTimeOutbound = true
					metric.UpdateOutboundTxNum(float64(oppyChain.Size()))
					oppyChain.OutboundReqChan <- outboundItems
					outBoundProcessDone.Store(false)
				}
			// process the in-bound top up event which will mint coin for users
			case itemsRecv := <-pi.InboundReqChan:
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer inBoundProcessDone.Store(true)
					processInbound(oppyGrpc, oppyChain, pi, itemsRecv, inBoundWait, failedInbound)
				}()

			case itemsRecv := <-oppyChain.OutboundReqChan:
				wg.Add(1)
				go func() {
					defer outBoundProcessDone.Store(true)
					defer wg.Done()
					processEachOutBound(oppyGrpc, oppyChain, pi, itemsRecv, failedOutbound, outBoundWait, &localSubmitLocker)
				}()

			case <-time.After(time.Second * 20):
				zlog.Logger.Info().Msgf("we should not reach here")
				err := pi.RetryPubChain()
				if err != nil {
					zlog.Logger.Error().Err(err).Msgf("fail to restart the pub chain")
				}
				err = oppyChain.RetryOppyChain()
				if err != nil {
					zlog.Logger.Error().Err(err).Msgf("fail to restart the oppy chain")
				}
			}
		}
	}(wg)
}

func processInbound(oppyGrpc string, oppyChain *oppybridge.OppyChainInstance, pi *pubchain.Instance, items []*common2.InBoundReq, inBoundWait *atomic.Bool, failedInbound *atomic.Int32) {
	grpcClient, err := grpc.Dial(oppyGrpc, grpc.WithInsecure())
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to dial the grpc end-point")
		return
	}
	defer grpcClient.Close()

	itemsMap := make(map[string]*common2.InBoundReq)
	for _, el := range items {
		itemsMap[el.Hash().Hex()] = el
	}

	needToBeProcessed := make([]*common2.InBoundReq, 0)
	for _, item := range items {
		// we need to check against the previous account sequence
		index := item.Hash().Hex()
		if oppyChain.CheckWhetherAlreadyExist(grpcClient, index) {
			zlog.Logger.Warn().Msg("already submitted by others")
			continue
		}
		needToBeProcessed = append(needToBeProcessed, item)
	}
	// if all the tx has been processed, we quit
	if len(needToBeProcessed) == 0 {
		failedInbound.Store(0)
		return
	}

	hashIndexMap, err := oppyChain.DoProcessInBound(grpcClient, needToBeProcessed)
	if err != nil {
		// we add all the txs back to wait list
		for _, el := range items {
			pi.AddOnHoldQueue(el)
		}
		return
	}

	wg := sync.WaitGroup{}
	for index, txHash := range hashIndexMap {
		wg.Add(1)
		go func(eachIndex, eachTxHash string) {
			defer wg.Done()

			grpcClientLocal, errLocal := grpc.Dial(oppyGrpc, grpc.WithInsecure())
			if errLocal != nil {
				zlog.Logger.Error().Err(errLocal).Msgf("fail to dial the grpc end-point")
				return
			}
			defer grpcClientLocal.Close()

			err := oppyChain.CheckTxStatus(grpcClientLocal, eachIndex, 20)
			if err != nil {
				zlog.Logger.Error().Err(err).Msgf("the tx index(%v) has not been successfully submitted retry", eachIndex)
				if !inBoundWait.Load() {
					failedInbound.Inc()
				}
				pi.AddOnHoldQueue(itemsMap[eachIndex])
				return
			}
			tick := html.UnescapeString("&#" + "128229" + ";")
			if eachTxHash == "" {
				failedInbound.Store(0)
				zlog.Logger.Info().Msgf("%v index(%v) have successfully top up by others", tick, eachIndex)
			} else {
				failedInbound.Store(0)
				zlog.Logger.Info().Msgf("%v txid(%v) have successfully top up", tick, eachTxHash)
			}
		}(index, txHash)
	}
	wg.Wait()
}

func processEachOutBound(oppyGrpc string, oppyChain *oppybridge.OppyChainInstance, pi *pubchain.Instance, items []*common2.OutBoundReq, failedOutBound *atomic.Int32, outBoundWait *atomic.Bool, localSubmitLocker *sync.Mutex) {
	checkWg := sync.WaitGroup{}
	needToBeProcessed := make([]*common2.OutBoundReq, 0)
	needToBeProcessedLock := sync.Mutex{}
	for _, el := range items {
		if el == nil {
			continue
		}
		checkWg.Add(1)
		go func(each *common2.OutBoundReq) {
			defer checkWg.Done()
			submittedTx, err := oppyChain.GetPubChainSubmittedTx(*each)
			if err != nil {
				zlog.Logger.Info().Msg("we continue process this tx as it has not been submitted")
				needToBeProcessedLock.Lock()
				needToBeProcessed = append(needToBeProcessed, each)
				needToBeProcessedLock.Unlock()
				return
			}

			if submittedTx != "" {
				zlog.Logger.Info().Msgf("we check whether someone has already submitted this tx %v", submittedTx)
				err := pi.CheckTxStatus(submittedTx)
				if err == nil {
					zlog.Logger.Info().Msg("this tx has been submitted by others, we skip it")
					return
				}
				needToBeProcessedLock.Lock()
				needToBeProcessed = append(needToBeProcessed, each)
				needToBeProcessedLock.Unlock()
			}
		}(el)
	}
	checkWg.Wait()
	if len(needToBeProcessed) == 0 {
		failedOutBound.Store(0)
		return
	}

	// now we process each tx
	emptyHash := common.Hash{}.Hex()
	tssWaitGroup := &sync.WaitGroup{}
	bc := pubchain.NewBroadcaster()
	tssReqChan := make(chan *pubchain.TssReq, len(needToBeProcessed))
	defer close(tssReqChan)
	for i, pItem := range needToBeProcessed {
		tssWaitGroup.Add(1)
		go func(index int, item *common2.OutBoundReq) {
			defer tssWaitGroup.Done()
			toAddr, fromAddr, tokenAddr, amount, nonce := item.GetOutBoundInfo()
			tssRespChan, err := bc.Subscribe(int64(index))
			if err != nil {
				panic("should not been subscribed!!")
			}
			defer bc.Unsubscribe(int64(index))
			txHash, err := pi.ProcessOutBound(index, toAddr, fromAddr, tokenAddr, amount, nonce, tssReqChan, tssRespChan)
			if err != nil {
				zlog.Logger.Error().Err(err).Msg("fail to broadcast the tx")
			}
			if txHash != emptyHash {
				err := pi.CheckTxStatus(txHash)
				if err == nil {
					failedOutBound.Store(0)
					tick := html.UnescapeString("&#" + "8599" + ";")
					zlog.Logger.Info().Msgf("%v we have send outbound tx(%v) from %v to %v (%v)", tick, txHash, fromAddr, toAddr, amount.String())
					// now we submit our public chain tx to oppychain
					localSubmitLocker.Lock()
					bf := backoff.NewExponentialBackOff()
					bf.MaxElapsedTime = time.Minute
					bf.MaxInterval = time.Second * 10
					op := func() error {
						grpcClient, err := grpc.Dial(oppyGrpc, grpc.WithInsecure())
						if err != nil {
							return err
						}
						defer grpcClient.Close()

						// we need to submit the pool created height as the validator may change in chain cosmos staking module
						// since we have started process the block, it is confirmed we have two pools
						pools := pi.GetPool()
						poolCreateHeight, err := strconv.ParseInt(pools[1].PoolInfo.BlockHeight, 10, 64)
						if err != nil {
							panic("blockheigh convert should never fail")
						}
						errInner := oppyChain.SubmitOutboundTx(grpcClient, nil, item.Hash().Hex(), poolCreateHeight, txHash, item.FeeToValidator)
						return errInner
					}
					err := backoff.Retry(op, bf)
					if err != nil {
						zlog.Logger.Error().Err(err).Msgf("we have tried but failed to submit the record with backoff")
					}
					localSubmitLocker.Unlock()
					return
				}
			}
			zlog.Logger.Warn().Msgf("the tx is fail in submission, we need to resend")
			if !outBoundWait.Load() {
				failedOutBound.Inc()
			}
			item.SubmittedTxHash = txHash
			oppyChain.AddOnHoldQueue(item)
		}(i, pItem)
	}
	// here we process the tss msg in batch
	tssWaitGroup.Add(1)
	go func() {
		defer tssWaitGroup.Done()
		var allsignMSgs [][]byte
		received := make(map[int][]byte)
		collected := false
		for {
			msg := <-tssReqChan
			received[msg.Index] = msg.Data
			if len(received) >= len(needToBeProcessed) {
				collected = true
			}
			if collected {
				break
			}
		}
		for _, val := range received {
			if bytes.Equal([]byte("none"), val) {
				continue
			}
			allsignMSgs = append(allsignMSgs, val)
		}

		lastPool := pi.GetPool()[1]
		latest, err := pi.GetBlockByNumberWithLock(nil)
		if err != nil {
			zlog.Logger.Error().Err(err).Msgf("fail to get the latest height")
			bc.Broadcast(nil)
			return
		}
		blockHeight := int64(latest.NumberU64()) / pubchain.ROUNDBLOCK
		signature, err := pi.TssSignBatch(allsignMSgs, lastPool.Pk, blockHeight)
		if err != nil {
			zlog.Info().Msgf("fail to run batch keysign")
		}
		bc.Broadcast(signature)
	}()
	tssWaitGroup.Wait()
}

func putOnHoldBlockInBoundBack(oppyGrpc string, pi *pubchain.Instance, oppyChain *oppybridge.OppyChainInstance) {
	grpcClient, err := grpc.Dial(oppyGrpc, grpc.WithInsecure())
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to dial the grpc end-point")
		return
	}
	defer grpcClient.Close()

	zlog.Logger.Debug().Msgf("we reload all the failed tx")
	itemInbound := pi.DumpQueue()
	wgDump := &sync.WaitGroup{}
	wgDump.Add(len(itemInbound))
	for _, el := range itemInbound {
		go func(each *common2.InBoundReq) {
			defer wgDump.Done()
			err := oppyChain.CheckTxStatus(grpcClient, each.Hash().Hex(), 2)
			if err == nil {
				tick := html.UnescapeString("&#" + "127866" + ";")
				zlog.Info().Msgf(" %v the tx has been submitted, we catch up with others on oppyChain", tick)
			} else {
				pi.AddItem(each)
			}
		}(el)
	}
	wgDump.Wait()
}

func putOnHoldBlockOutBoundBack(oppyGrpc string, pi *pubchain.Instance, oppyChain *oppybridge.OppyChainInstance) {
	grpcClient, err := grpc.Dial(oppyGrpc, grpc.WithInsecure())
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to dial the grpc end-point")
		return
	}
	defer grpcClient.Close()

	zlog.Logger.Debug().Msgf("we reload all the failed tx")
	itemsOutBound := oppyChain.DumpQueue()
	wgDump := &sync.WaitGroup{}
	wgDump.Add(len(itemsOutBound))
	for _, el := range itemsOutBound {
		go func(each *common2.OutBoundReq) {
			defer wgDump.Done()
			empty := common.Hash{}.Hex()
			if each.SubmittedTxHash == empty {
				oppyChain.AddItem(each)
				return
			}
			err := pi.CheckTxStatus(each.SubmittedTxHash)
			if err != nil {
				oppyChain.AddItem(each)
				return
			}
			tick := html.UnescapeString("&#" + "127866" + ";")
			zlog.Info().Msgf(" %v the tx has been submitted, we catch up with others on pubchain", tick)
		}(el)
	}
	wgDump.Wait()
}

func updateHealthCheck(pi *pubchain.Instance, metric *monitor.Metric) {
	err := pi.CheckPubChainHealthWithLock()
	if err != nil {
		return
	}
	metric.UpdateStatus()
}
