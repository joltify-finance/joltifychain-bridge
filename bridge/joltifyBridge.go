package bridge

import (
	"context"
	"fmt"
	"html"
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"os/signal"
	"path"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"gitlab.com/joltify/joltifychain-bridge/storage"

	"github.com/ethereum/go-ethereum/common"

	common2 "gitlab.com/joltify/joltifychain-bridge/common"

	"gitlab.com/joltify/joltifychain-bridge/monitor"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"

	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/joltifybridge"
	"gitlab.com/joltify/joltifychain-bridge/pubchain"

	zlog "github.com/rs/zerolog/log"

	"github.com/tendermint/tendermint/types"
)

// NewBridgeService starts the new bridge service
func NewBridgeService(config config.Config) {
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
		cancel()
		return
	}

	metrics := monitor.NewMetric()
	if config.EnableMonitor {
		metrics.Enable()
	}

	// fixme, in docker it needs to be changed to basehome
	tssServer, _, err := tssclient.StartTssServer(config.HomeDir, config.TssConfig)
	if err != nil {
		log.Fatalln("fail to start the tss")
		cancel()
		return
	}

	joltifyBridge, err := joltifybridge.NewJoltifyBridge(config.JoltifyChain.GrpcAddress, config.JoltifyChain.WsAddress, tssServer)
	if err != nil {
		log.Fatalln("fail to create the invoice joltify_bridge", err)
		cancel()
		return
	}

	keyringPath := path.Join(config.HomeDir, config.KeyringAddress)

	dat, err := ioutil.ReadFile(keyringPath)
	if err != nil {
		log.Fatalln("error in read keyring file")
		cancel()
		return
	}

	// fixme need to update the passcode
	err = joltifyBridge.Keyring.ImportPrivKey("operator", string(dat), "12345678")
	if err != nil {
		cancel()
		return
	}

	defer func() {
		err := joltifyBridge.TerminateBridge()
		if err != nil {
			return
		}
	}()

	err = joltifyBridge.InitValidators(config.JoltifyChain.HTTPAddress)
	if err != nil {
		fmt.Printf("error in init the validators %v", err)
		cancel()
		return
	}
	tssHTTPServer := NewJoltifyHttpServer(ctx, config.TssConfig.HTTPAddr, joltifyBridge.GetTssNodeID())

	wg.Add(1)
	ret := tssHTTPServer.Start(&wg)
	if ret != nil {
		cancel()
		return
	}

	// now we monitor the bsc transfer event
	pi, err := pubchain.NewChainInstance(config.PubChainConfig.WsAddress, tssServer)
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
			joltifyBridge.AddItem(el)
		}
		fmt.Printf("we have loaded the unprocessed outbound tx")
	}

	// now we load the existing inbound requests
	itemsIn, err := fsm.LoadInBoundState()
	if err != nil {
		fmt.Printf("we do not need to have the items to be loaded")
	}
	if itemsIn != nil {
		for _, el := range itemsIn {
			pi.AddItem(el)
		}
		fmt.Printf("we have loaded the unprocessed inbound tx")
	}

	pfsm := storage.NewPendingTxStateMgr(config.HomeDir)
	// now we load the existing inbound pending txs
	inBoundPendingTx, err := pfsm.LoadPendingItems()
	if err != nil {
		fmt.Printf("we do not need to have the pending tx to be loaded")
	}
	if inBoundPendingTx != nil {
		for _, el := range inBoundPendingTx {
			pi.AddPendingTx(el)
		}
		fmt.Printf("we have loaded the unprocessed inbound pending tx")
	}

	inBoundPendingBnbTx, err := pfsm.LoadPendingBnbItems()
	if err != nil {
		fmt.Printf("we do not need to have the pending bnb tx to be loded")
	}
	if inBoundPendingBnbTx != nil {
		for _, el := range inBoundPendingBnbTx {
			pi.AddPendingTxBnb(el)
		}
		fmt.Printf("we have loaded the unprocessed inbound bnb pending tx")
	}

	addEventLoop(ctx, &wg, joltifyBridge, pi, metrics, fsm, config)
	<-c
	cancel()
	wg.Wait()

	itemsexported := joltifyBridge.ExportItems()
	err = fsm.SaveOutBoundState(itemsexported)
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to save the outbound requests!!!")
	}

	itemsInexported := pi.ExportItems()
	err = fsm.SaveInBoundState(itemsInexported)
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to save the outbound requests!!!")
	}

	zlog.Info().Msgf("we have saved the unprocessed outbound txs")

	pendingitemsexported := pi.ExportPendingItems()
	err = pfsm.SavePendingItems(pendingitemsexported)
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to save the pending tx!!!")
	}

	pendingbnbitemsexported := pi.ExportPendingBnbItems()
	err = pfsm.SavePendingBnbItems(pendingbnbitemsexported)
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to save the pending bnb tx!!!")
	}

	zlog.Info().Msgf("we have saved the unprocessed inbound pending txs")

	zlog.Logger.Info().Msgf("we quit the bridge gracefully")
}

func addEventLoop(ctx context.Context, wg *sync.WaitGroup, joltChain *joltifybridge.JoltifyChainInstance, pi *pubchain.Instance, metric *monitor.Metric, fsm *storage.TxStateMgr, providedConfig config.Config) {
	query := "tm.event = 'ValidatorSetUpdates'"
	ctxLocal, cancelLocal := context.WithTimeout(context.Background(), time.Second*5)
	defer cancelLocal()

	validatorUpdateChan, err := joltChain.AddSubscribe(ctxLocal, query)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return
	}

	query = "tm.event = 'NewBlock'"
	newJoltifyBlock, err := joltChain.AddSubscribe(ctxLocal, query)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return
	}

	// pubNewBlockChan is the channel for the new blocks for the public chain
	subscriptionCtx, cancelsubscription := context.WithCancel(context.Background())
	wg.Add(1)
	pubNewBlockChan, err := pi.StartSubscription(subscriptionCtx, wg)
	if err != nil {
		fmt.Printf("fail to subscribe the token transfer with err %v\n", err)
		cancelsubscription()
		return
	}
	blockHeight, err := joltChain.GetLastBlockHeight()
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
			case vals := <-validatorUpdateChan:
				height, err := joltChain.GetLastBlockHeight()
				if err != nil {
					continue
				}
				validatorUpdates := vals.Data.(types.EventDataValidatorSetUpdates).ValidatorUpdates
				err = joltChain.HandleUpdateValidators(validatorUpdates, height)
				if err != nil {
					fmt.Printf("error in handle update validator")
					continue
				}

			// process the new joltify block, validator may need to submit the pool address
			case block := <-newJoltifyBlock:
				currentBlockHeight := block.Data.(types.EventDataNewBlock).Block.Height
				ok, _ := joltChain.CheckAndUpdatePool(currentBlockHeight)
				if !ok {
					// it is okay to fail to submit a pool address as other nodes can submit, as long as 2/3 nodes submit, it is fine.
					zlog.Logger.Warn().Msgf("we fail to submit the new pool address")
				}

				joltChain.CurrentHeight = currentBlockHeight

				// we update the tx new, if there exits a processable block
				if currentBlockHeight > int64(providedConfig.JoltifyChain.RollbackGap) {
					processableBlockHeight := currentBlockHeight - int64(providedConfig.JoltifyChain.RollbackGap)
					processableBlock, err := joltChain.GetBlockByHeight(processableBlockHeight)
					if err != nil {
						zlog.Logger.Error().Err(err).Msgf("error in get block to process %v", err)
						continue
					}
					for _, el := range processableBlock.Data.Txs {
						joltChain.CheckOutBoundTx(processableBlockHeight, el)
					}

				}

				// now we check whether we need to update the pool
				// we query the pool from the chain directly.
				poolInfo, err := joltChain.QueryLastPoolAddress()
				if err != nil {
					zlog.Logger.Error().Err(err).Msgf("error in get pool with error %v", err)
					continue
				}
				if len(poolInfo) != 2 {
					zlog.Logger.Warn().Msgf("the pool only have %v address, bridge will not work", len(poolInfo))
					continue
				}

				currentPool := pi.GetPool()
				// this means the pools has not been filled with two address
				if currentPool[0] == nil {
					for _, el := range poolInfo {
						err := pi.UpdatePool(el)
						if err != nil {
							zlog.Log().Err(err).Msgf("fail to update the pool")
						}
						joltChain.UpdatePool(el)
					}
					continue
				}

				if NeedUpdate(poolInfo, currentPool) {
					err := pi.UpdatePool(poolInfo[0])
					if err != nil {
						zlog.Log().Err(err).Msgf("fail to update the pool")
					}
					previousPool := joltChain.UpdatePool(poolInfo[0])
					if previousPool.Pk != poolInfo[0].CreatePool.PoolPubKey {
						// we force the first try of the tx to be run without blocking by the block wait
						joltChain.AddMoveFundItem(previousPool, currentBlockHeight-config.MINCHECKBLOCKGAP+5)
						pi.AddMoveFundItem(previousPool, pi.CurrentHeight-config.MINCHECKBLOCKGAP+5)
					}
				}

				latestPool := poolInfo[0].CreatePool.PoolAddr
				moveFound := joltChain.MoveFound(currentBlockHeight, latestPool)
				if moveFound {
					// if we move fund in this round, we do not send tx to sign
					continue
				}

				// todo we need also to add the check to avoid send tx near the churn blocks
				if currentBlockHeight-previousTssBlockInbound >= pubchain.GroupBlockGap && pi.Size() != 0 {
					// if we do not have enough tx to process, we wait for another round
					if pi.Size() < pubchain.GroupSign && firstTimeInbound {
						firstTimeInbound = false
						metric.UpdateInboundTxNum(float64(pi.Size()))
						continue
					}

					pools := joltChain.GetPool()
					zlog.Logger.Warn().Msgf("we feed the tx now %v", pools[1].PoolInfo.CreatePool.String())
					err := joltChain.FeedTx(pools[1].PoolInfo, pi, currentBlockHeight)
					if err != nil {
						zlog.Logger.Error().Err(err).Msgf("fail to feed the tx")
					}
					previousTssBlockInbound = currentBlockHeight
					firstTimeInbound = true
					metric.UpdateInboundTxNum(float64(pi.Size()))
				}

				// process the public chain new block event
			case head := <-pubNewBlockChan:
				joltifyBlockHeight, errInner := joltChain.GetLastBlockHeight()
				if errInner != nil {
					zlog.Logger.Error().Err(err).Msg("fail to get the joltify chain block height for this round")
					continue
				}
				// process block with rollback gap
				var processableBlockHeight = big.NewInt(0).Sub(head.Number, big.NewInt(int64(providedConfig.PubChainConfig.RollbackGap)))
				err := pi.ProcessNewBlock(processableBlockHeight, joltifyBlockHeight)
				pi.CurrentHeight = head.Number.Int64()
				if err != nil {
					zlog.Logger.Error().Err(err).Msg("fail to process the inbound block")
				}
				// we delete the expired tx
				pi.DeleteExpired(head.Number.Uint64())

				isMoveFund := false
				previousPool, _ := pi.PopMoveFundItemAfterBlock(head.Number.Int64())
				isSigner := false
				if previousPool != nil {
					isSigner, err = joltChain.CheckWhetherSigner(previousPool.PoolInfo)
					if err != nil {
						zlog.Logger.Warn().Msg("fail in check whether we are signer in moving fund")
					}
				}

				if isSigner && previousPool != nil {
					// we move fund in the public chain
					isMoveFund = pi.MoveFound(wg, head.Number.Int64(), previousPool)
				}
				if isMoveFund {
					// once we move fund, we do not send tx to be processed
					continue
				}

				// todo we need also to add the check to avoid send tx near the churn blocks
				if joltifyBlockHeight-previousTssBlockOutBound >= joltifybridge.GroupBlockGap && joltChain.Size() != 0 {
					// if we do not have enough tx to process, we wait for another round
					if joltChain.Size() < pubchain.GroupSign && firstTimeOutbound {
						firstTimeOutbound = false
						metric.UpdateOutboundTxNum(float64(joltChain.Size()))
						continue
					}

					pools := joltChain.GetPool()
					if len(pools) < 2 || pools[1] == nil {
						// this is need once we resume the bridge to avoid the panic that the pool address has not been filled
						zlog.Logger.Warn().Msgf("we do not have 2 pools to start the tx")
						continue
					}
					zlog.Logger.Warn().Msgf("we feed the tx now %v", pools[1].PoolInfo.CreatePool.String())

					outboundItems := joltChain.PopItem(pubchain.GroupSign)

					if outboundItems == nil {
						zlog.Logger.Info().Msgf("empty queue")
						continue
					}

					found, err := joltChain.CheckWhetherSigner(pools[1].PoolInfo)
					if err != nil {
						zlog.Logger.Error().Err(err).Msg("fail to check whether we are the node submit the mint request")
						continue
					}
					if !found {
						zlog.Logger.Info().Msgf("we are not the signer")
						continue
					}

					err = pi.FeedTx(joltifyBlockHeight, pools[1].PoolInfo, outboundItems)
					if err != nil {
						zlog.Logger.Error().Err(err).Msgf("fail to feed the tx")
						continue
					}
					previousTssBlockOutBound = joltifyBlockHeight
					firstTimeOutbound = true
					metric.UpdateOutboundTxNum(float64(joltChain.Size()))

					for _, el := range outboundItems {
						joltChain.OutboundReqChan <- el
					}
				}

			// process the in-bound top up event which will mint coin for users
			case itemRecv := <-pi.InboundReqChan:
				wg.Add(1)
				go func(item *common2.InBoundReq) {
					defer wg.Done()
					txHash, indexRet, err := joltChain.ProcessInBound(item)
					if txHash == "" && indexRet == "" && err == nil {
						return
					}
					wg.Add(1)
					go func(index string) {
						defer wg.Done()
						err := joltChain.CheckTxStatus(index)
						if err != nil {
							zlog.Logger.Error().Err(err).Msgf("the tx index(%v) has not been successfully submitted retry", index)
							pi.AddItem(item)
							return
						}
						tick := html.UnescapeString("&#" + "128229" + ";")
						if txHash == "" {
							zlog.Logger.Info().Msgf("%v index(%v) have successfully top up by others", tick, index)
						} else {
							zlog.Logger.Info().Msgf("%v txid(%v) have successfully top up", tick, txHash)
						}
					}(indexRet)
				}(itemRecv)

			case itemRecv := <-joltChain.OutboundReqChan:

				wg.Add(1)
				go func(litem *common2.OutBoundReq) {
					defer wg.Done()
					submittedTx, err := joltChain.GetPubChainSubmittedTx(*litem)
					if err != nil {
						zlog.Logger.Error().Err(err).Msg("fail to query the submitted tx record, we continue process this tx")
					}

					if submittedTx != "" {
						zlog.Logger.Info().Msgf("we check whether someone has already submitted this tx %v", submittedTx)
						err := pi.CheckTxStatus(submittedTx)
						if err == nil {
							zlog.Logger.Info().Msg("this tx has been submitted by others, we skip it")
							return
						}
					}

					toAddr, fromAddr, tokenAddr, amount, roundBlockHeight, nonce := litem.GetOutBoundInfo()
					txHashCheck, err := pi.ProcessOutBound(wg, toAddr, fromAddr, tokenAddr, amount, roundBlockHeight, nonce)
					if err != nil {
						zlog.Logger.Error().Err(err).Msg("fail to broadcast the tx")
					}
					// though we submit the tx successful, we may still fail as tx may run out of gas,so we need to check
					wg.Add(1)
					go func(itemCheck *common2.OutBoundReq, txHash string) {
						defer wg.Done()
						emptyHash := common.Hash{}.Hex()
						if txHash != emptyHash {
							err := pi.CheckTxStatus(txHash)
							if err == nil {
								tick := html.UnescapeString("&#" + "128228" + ";")
								zlog.Logger.Info().Msgf("%v we have send outbound tx(%v) from %v to %v (%v)", tick, txHash, fromAddr, toAddr, amount.String())
								// now we submit our public chain tx to joltifychain
								localSubmitLocker.Lock()
								bf := backoff.NewExponentialBackOff()
								bf.MaxElapsedTime = time.Minute
								bf.MaxInterval = time.Second * 10
								op := func() error {
									errInner := joltChain.SubmitOutboundTx(itemCheck.Hash().Hex(), itemCheck.OriginalHeight, txHash)
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
						joltChain.AddItem(itemCheck)
					}(litem, txHashCheck)
				}(itemRecv)
			}
		}
	}(wg)
}
