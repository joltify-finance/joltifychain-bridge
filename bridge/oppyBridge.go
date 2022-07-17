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
	"strconv"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tendermint/tendermint/types"
	"gitlab.com/oppy-finance/oppy-bridge/storage"
	"gitlab.com/oppy-finance/oppy-bridge/tokenlist"

	"github.com/ethereum/go-ethereum/common"

	common2 "gitlab.com/oppy-finance/oppy-bridge/common"

	"gitlab.com/oppy-finance/oppy-bridge/monitor"
	"gitlab.com/oppy-finance/oppy-bridge/tssclient"

	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/oppybridge"
	"gitlab.com/oppy-finance/oppy-bridge/pubchain"

	zlog "github.com/rs/zerolog/log"
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
		cancel()
		return
	}

	oppyBridge, err := oppybridge.NewOppyBridge(config.OppyChain.GrpcAddress, config.OppyChain.WsAddress, tssServer, tl)
	if err != nil {
		log.Fatalln("fail to create the invoice oppy_bridge", err)
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
	err = oppyBridge.Keyring.ImportPrivKey("operator", string(dat), "12345678")
	if err != nil {
		cancel()
		return
	}

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
	tssHTTPServer := NewOppyHttpServer(ctx, config.TssConfig.HTTPAddr, oppyBridge.GetTssNodeID())

	wg.Add(1)
	ret := tssHTTPServer.Start(&wg)
	if ret != nil {
		cancel()
		return
	}

	// now we monitor the bsc transfer event
	pi, err := pubchain.NewChainInstance(config.PubChainConfig.WsAddress, tssServer, tl)
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

	// now we load the pending outboundtx
	pendingManager := storage.NewPendingTxStateMgr(config.HomeDir)
	pendingItems, err := pendingManager.LoadPendingItems()
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to load the pending items!!")
	}

	if len(pendingItems) != 0 {
		oppyBridge.Import(pendingItems)
	}

	addEventLoop(ctx, &wg, oppyBridge, pi, metrics, fsm, int64(config.OppyChain.RollbackGap), int64(config.PubChainConfig.RollbackGap), tl, config.PubChainConfig.WsAddress)
	<-c
	cancel()
	wg.Wait()

	itemsexported := oppyBridge.ExportItems()
	err = fsm.SaveOutBoundState(itemsexported)
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to save the outbound requests!!!")
	}

	itemsInexported := pi.ExportItems()
	err = fsm.SaveInBoundState(itemsInexported)
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to save the outbound requests!!!")
	}

	exportedPending := oppyBridge.Export()

	err = pendingManager.SavePendingItems(exportedPending)
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to save the pending requests!!!")
	}

	zlog.Logger.Info().Msgf("we quit the bridge gracefully")
}

func addEventLoop(ctx context.Context, wg *sync.WaitGroup, oppyChain *oppybridge.OppyChainInstance, pi *pubchain.Instance, metric *monitor.Metric, fsm *storage.TxStateMgr, oppyRollbackGap int64, pubRollbackGap int64, tl *tokenlist.TokenList, pubChainWsAddress string) {
	query := "complete_churn.churn = 'oppy_churn'"
	ctxLocal, cancelLocal := context.WithTimeout(context.Background(), time.Second*5)
	defer cancelLocal()

	validatorUpdateChan, err := oppyChain.AddSubscribe(ctxLocal, query)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return
	}

	query = "tm.event = 'NewBlock'"
	newOppyBlock, err := oppyChain.AddSubscribe(ctxLocal, query)
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
	blockHeight, err := oppyChain.GetLastBlockHeight()
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
				height, err := oppyChain.GetLastBlockHeight()
				if err != nil {
					continue
				}

				_, ok := vals.Data.(types.EventDataNewBlock)
				if !ok {
					continue
				}

				err = oppyChain.HandleUpdateValidators(height)
				if err != nil {
					fmt.Printf("error in handle update validator")
					continue
				}

			// process the new oppy block, validator may need to submit the pool address
			case block := <-newOppyBlock:
				currentBlockHeight := block.Data.(types.EventDataNewBlock).Block.Height
				ok, _ := oppyChain.CheckAndUpdatePool(currentBlockHeight)
				if !ok {
					// it is okay to fail to submit a pool address as other nodes can submit, as long as 2/3 nodes submit, it is fine.
					zlog.Logger.Warn().Msgf("we fail to submit the new pool address")
				}

				oppyChain.CurrentHeight = currentBlockHeight

				// we update the token list, if the current block height refresh the update mark
				err := tl.UpdateTokenList(oppyChain.CurrentHeight)
				if err != nil {
					zlog.Logger.Warn().Msgf("error in updating token list %v", err)
				}

				if currentBlockHeight%pubchain.PRICEUPDATEGAP == 0 {
					price, err := pi.GetGasPriceWithLock()
					if err == nil {
						oppyChain.UpdatePubChainGasPrice(price.Int64())
					} else {
						zlog.Logger.Error().Err(err).Msg("fail to get the suggest gas price")
					}
				}

				// we update the tx new, if there exits a processable block
				if currentBlockHeight > oppyRollbackGap {
					processableBlockHeight := currentBlockHeight - oppyRollbackGap
					processableBlock, err := oppyChain.GetBlockByHeight(processableBlockHeight)
					if err != nil {
						zlog.Logger.Error().Err(err).Msgf("error in get block to process %v", err)
						continue
					}
					oppyChain.DeleteExpired(currentBlockHeight)

					// here we process the outbound tx
					for _, el := range processableBlock.Data.Txs {
						oppyChain.CheckOutBoundTx(processableBlockHeight, el)
					}
				}

				// now we check whether we need to update the pool
				// we query the pool from the chain directly.
				poolInfo, err := oppyChain.QueryLastPoolAddress()
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
						oppyChain.UpdatePool(el)
					}
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
						oppyChain.AddMoveFundItem(previousPool, currentBlockHeight-config.MINCHECKBLOCKGAP+5)
						pi.AddMoveFundItem(previousPool, pi.CurrentHeight-config.MINCHECKBLOCKGAP+5)
					}
				}

				latestPool := poolInfo[0].CreatePool.PoolAddr
				moveFound := oppyChain.MoveFound(currentBlockHeight, latestPool)
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

					pools := oppyChain.GetPool()
					zlog.Logger.Warn().Msgf("we feed the tx now %v", pools[1].PoolInfo.CreatePool.String())
					err := oppyChain.FeedTx(pools[1].PoolInfo, pi, currentBlockHeight)
					if err != nil {
						zlog.Logger.Error().Err(err).Msgf("fail to feed the tx")
					}
					previousTssBlockInbound = currentBlockHeight
					firstTimeInbound = true
					metric.UpdateInboundTxNum(float64(pi.Size()))
				}

				// process the public chain new block event
			case head := <-pubNewBlockChan:
				oppyBlockHeight, errInner := oppyChain.GetLastBlockHeight()
				if errInner != nil {
					zlog.Logger.Error().Err(err).Msg("fail to get the oppy chain block height for this round")
					continue
				}
				// process block with rollback gap
				processableBlockHeight := big.NewInt(0).Sub(head.Number, big.NewInt(pubRollbackGap))
				err := pi.ProcessNewBlock(processableBlockHeight, oppyBlockHeight)
				pi.CurrentHeight = head.Number.Int64()
				if err != nil {
					zlog.Logger.Error().Err(err).Msg("fail to process the inbound block")
				}

				isMoveFund := false
				previousPool, height := pi.PopMoveFundItemAfterBlock(head.Number.Int64())
				isSigner := false
				if previousPool != nil {
					isSigner, err = oppyChain.CheckWhetherSigner(previousPool.PoolInfo)
					if err != nil {
						zlog.Logger.Warn().Msg("fail in check whether we are signer in moving fund")
					}
				}

				if isSigner && previousPool != nil {
					// we move fund in the public chain
					ethClient, err := ethclient.Dial(pubChainWsAddress)
					if err != nil {
						pi.AddMoveFundItem(previousPool, height)
						zlog.Logger.Error().Err(err).Msg("fail to dial the websocket")
					}
					if ethClient != nil {
						isMoveFund = pi.MoveFound(wg, head.Number.Int64(), previousPool, height, ethClient)
					}
					ethClient.Close()
				}
				if isMoveFund {
					// once we move fund, we do not send tx to be processed
					continue
				}

				// todo we need also to add the check to avoid send tx near the churn blocks
				if oppyBlockHeight-previousTssBlockOutBound >= oppybridge.GroupBlockGap && oppyChain.Size() != 0 {
					// if we do not have enough tx to process, we wait for another round
					if oppyChain.Size() < pubchain.GroupSign && firstTimeOutbound {
						firstTimeOutbound = false
						metric.UpdateOutboundTxNum(float64(oppyChain.Size()))
						continue
					}

					pools := oppyChain.GetPool()
					if len(pools) < 2 || pools[1] == nil {
						// this is need once we resume the bridge to avoid the panic that the pool address has not been filled
						zlog.Logger.Warn().Msgf("we do not have 2 pools to start the tx")
						continue
					}
					zlog.Logger.Warn().Msgf("we feed the tx now %v", pools[1].PoolInfo.CreatePool.String())

					outboundItems := oppyChain.PopItem(pubchain.GroupSign)

					if outboundItems == nil {
						zlog.Logger.Info().Msgf("empty queue")
						continue
					}

					found, err := oppyChain.CheckWhetherSigner(pools[1].PoolInfo)
					if err != nil {
						zlog.Logger.Error().Err(err).Msg("fail to check whether we are the node submit the mint request")
						continue
					}
					if !found {
						zlog.Logger.Info().Msgf("we are not the signer")
						continue
					}

					err = pi.FeedTx(oppyBlockHeight, pools[1].PoolInfo, outboundItems)
					if err != nil {
						zlog.Logger.Error().Err(err).Msgf("fail to feed the tx")
						continue
					}
					previousTssBlockOutBound = oppyBlockHeight
					firstTimeOutbound = true
					metric.UpdateOutboundTxNum(float64(oppyChain.Size()))

					for _, el := range outboundItems {
						oppyChain.OutboundReqChan <- el
					}
				}

			// process the in-bound top up event which will mint coin for users
			case itemRecv := <-pi.InboundReqChan:
				wg.Add(1)
				go func(item *common2.InBoundReq) {
					defer wg.Done()
					txHash, indexRet, err := oppyChain.ProcessInBound(item)
					if txHash == "" && indexRet == "" && err == nil {
						return
					}
					wg.Add(1)
					go func(index string) {
						defer wg.Done()
						err := oppyChain.CheckTxStatus(index)
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

			case itemRecv := <-oppyChain.OutboundReqChan:

				wg.Add(1)
				go func(litem *common2.OutBoundReq) {
					defer wg.Done()
					submittedTx, err := oppyChain.GetPubChainSubmittedTx(*litem)
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
								// now we submit our public chain tx to oppychain
								localSubmitLocker.Lock()
								bf := backoff.NewExponentialBackOff()
								bf.MaxElapsedTime = time.Minute
								bf.MaxInterval = time.Second * 10
								op := func() error {
									// we need to submit the pool created height as the validator may change in chain cosmos staking module
									// since we have started process the block, it is confirmed we have two pools
									pools := pi.GetPool()
									poolCreateHeight, err := strconv.ParseInt(pools[1].PoolInfo.BlockHeight, 10, 64)
									if err != nil {
										panic("blockheigh convert should never fail")
									}
									errInner := oppyChain.SubmitOutboundTx(nil, itemCheck.Hash().Hex(), poolCreateHeight, txHash)
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
						oppyChain.AddItem(itemCheck)
					}(litem, txHashCheck)
				}(itemRecv)
			}
		}
	}(wg)
}
