package bridge

import (
	"context"
	"fmt"
	"github.com/cenkalti/backoff"
	"html"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"

	common2 "gitlab.com/joltify/joltifychain-bridge/common"

	"gitlab.com/joltify/joltifychain-bridge/monitor"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"

	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/joltifybridge"
	"gitlab.com/joltify/joltifychain-bridge/pubchain"

	zlog "github.com/rs/zerolog/log"

	tmtypes "github.com/tendermint/tendermint/types"
)

const blockCache = 512

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
	ci, err := pubchain.NewChainInstance(config.PubChainConfig.WsAddress, config.PubChainConfig.TokenAddress, tssServer)
	if err != nil {
		fmt.Printf("fail to connect the public pub_chain with address %v\n", config.PubChainConfig.WsAddress)
		cancel()
		return
	}

	addEventLoop(ctx, &wg, joltifyBridge, ci, metrics)
	<-c
	cancel()
	wg.Wait()
	cancel()
	zlog.Logger.Info().Msgf("we quit the bridge gracefully")
}

func addEventLoop(ctx context.Context, wg *sync.WaitGroup, joltChain *joltifybridge.JoltifyChainInstance, pi *pubchain.Instance, metric *monitor.Metric) {
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

	currentBlock := make([]*ctypes.ResultEvent, 1)

	// pubNewBlockChan is the channel for the new blocks for the public chain
	subscriptionCtx, cancelsubscription := context.WithCancel(context.Background())
	wg.Add(1)
	pubNewBlockChan, err := pi.StartSubscription(subscriptionCtx, wg)
	if err != nil {
		fmt.Printf("fail to subscribe the token transfer with err %v\n", err)
		cancelsubscription()
		return
	}
	previousTssBlockInbound := int64(0)
	previousTssBlockOutBound := int64(0)
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
				validatorUpdates := vals.Data.(tmtypes.EventDataValidatorSetUpdates).ValidatorUpdates
				err = joltChain.HandleUpdateValidators(validatorUpdates, height)
				if err != nil {
					fmt.Printf("error in handle update validator")
					continue
				}

			// process the new joltify block, validator may need to submit the pool address
			case block := <-newJoltifyBlock:
				currentBlockHeight := block.Data.(tmtypes.EventDataNewBlock).Block.Height
				ok, _ := joltChain.CheckAndUpdatePool(currentBlockHeight)
				if !ok {
					// it is okay to fail to submit a pool address as other nodes can submit, as long as 2/3 nodes submit, it is fine.
					zlog.Logger.Warn().Msgf("we fail to submit the new pool address")
				}

				joltChain.CurrentHeight = currentBlockHeight

				// we update the tx new, since the tx is committed in the next block, we need to handle the tx once the next block coms
				if currentBlock[0] == nil {
					currentBlock[0] = &block
				} else {
					previousBlock := currentBlock[0]
					currentBlock[0] = &block
					preBlockHeight := previousBlock.Data.(tmtypes.EventDataNewBlock).Block.Height
					for _, el := range previousBlock.Data.(tmtypes.EventDataNewBlock).Block.Txs {
						joltChain.CheckOutBoundTx(preBlockHeight, el)
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
				err := pi.ProcessNewBlock(head.Number, joltifyBlockHeight)
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
					zlog.Logger.Warn().Msgf("we feed the tx now %v", pools[1].PoolInfo.CreatePool.String())

					found, err := joltChain.CheckWhetherSigner(pools[1].PoolInfo)
					if err != nil {
						zlog.Logger.Error().Err(err).Msg("fail to check whether we are the node submit the mint request")
						continue
					}
					if !found {
						zlog.Logger.Info().Msgf("we are not the signer")
						continue
					}

					outboundItems := joltChain.PopItem(pubchain.GroupSign)

					if outboundItems == nil {
						zlog.Logger.Info().Msgf("empty queue")
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
			case item := <-pi.InboundReqChan:
				wg.Add(1)
				go func() {
					defer wg.Done()
					txHash, index, _ := joltChain.ProcessInBound(item)
					wg.Add(1)
					go func() {
						defer wg.Done()
						err := joltChain.CheckTxStatus(index)
						if err != nil {
							zlog.Logger.Error().Err(err).Msgf("the tx has not been successfully submitted retry")
							pi.AddItem(item)
						}
						tick := html.UnescapeString("&#" + "128229" + ";")
						if txHash == "" {
							zlog.Logger.Info().Msgf("%v index(%v) have successfully top up by others", tick, index)
						} else {
							zlog.Logger.Info().Msgf("%v txid(%v) have successfully top up", tick, txHash)
						}
					}()
				}()

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

					toAddr, fromAddr, amount, roundBlockHeight, nonce := litem.GetOutBoundInfo()
					txHashCheck, err := pi.ProcessOutBound(wg, toAddr, fromAddr, amount, roundBlockHeight, nonce)
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
