package bridge

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path"
	"sync"
	"time"

	"gitlab.com/joltify/joltifychain-bridge/tssclient"

	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/joltifybridge"
	"gitlab.com/joltify/joltifychain-bridge/pubchain"

	zlog "github.com/rs/zerolog/log"

	tmtypes "github.com/tendermint/tendermint/types"
)

// NewBridgeService starts the new bridge service
func NewBridgeService(config config.Config) {
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	passcodeLength := 32
	passcode := make([]byte, passcodeLength)
	n, err := os.Stdin.Read(passcode)
	if err != nil {
		return
	}
	if n > passcodeLength {
		log.Fatalln("the passcode is too long")
		return
	}

	// fixme, in docker it needs to be changed to basehome
	tssServer, _, err := tssclient.StartTssServer(config.HomeDir, config.TssConfig)
	if err != nil {
		log.Fatalln("fail to start the tss")
		return
	}

	joltifyBridge, err := joltifybridge.NewJoltifyBridge(config.InvoiceChainConfig.GrpcAddress, config.InvoiceChainConfig.HttpAddress, tssServer)
	if err != nil {
		log.Fatalln("fail to create the invoice joltify_bridge", err)
		return
	}

	keyringPath := path.Join(config.HomeDir, config.KeyringAddress)

	dat, err := ioutil.ReadFile(keyringPath)
	if err != nil {
		log.Fatalln("error in read keyring file")
		return
	}

	// fixme need to update the passcode
	err = joltifyBridge.Keyring.ImportPrivKey("operator", string(dat), "12345678")
	if err != nil {
		return
	}

	defer func() {
		err := joltifyBridge.TerminateBridge()
		if err != nil {
			return
		}
	}()

	err = joltifyBridge.InitValidators(config.InvoiceChainConfig.RPCAddress)
	if err != nil {
		fmt.Printf("error in init the validators %v", err)
		cancel()
		return
	}
	tssHTTPServer := NewTssHttpServer(config.TssConfig.HttpAddr, joltifyBridge.GetTssNodeID(), ctx)

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

	wg.Add(1)
	addEventLoop(ctx, &wg, joltifyBridge, ci)

	<-c
	ctx.Done()
	cancel()
	wg.Wait()
	fmt.Printf("we quit gracefully\n")
}

func addEventLoop(ctx context.Context, wg *sync.WaitGroup, joltChain *joltifybridge.JoltifyChainInstance, pi *pubchain.PubChainInstance) {
	defer wg.Done()
	query := "tm.event = 'ValidatorSetUpdates'"
	ctxLocal, cancelLocal := context.WithTimeout(ctx, time.Second*5)
	defer cancelLocal()

	validatorUpdateChan, err := joltChain.AddSubscribe(ctxLocal, query)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return
	}

	query = "tm.event = 'NewBlock'"
	newBlockChan, err := joltChain.AddSubscribe(ctxLocal, query)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return
	}

	query = "tm.event = 'Tx'"

	newJoltifyTxChan, err := joltChain.AddSubscribe(ctxLocal, query)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return
	}

	wg.Add(1)

	// pubNewBlockChan is the channel for the new blocks for the public chain
	pubNewBlockChan, err := pi.StartSubscription(ctx, wg)
	if err != nil {
		fmt.Printf("fail to subscribe the token transfer with err %v\n", err)
		return
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
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
			case block := <-newBlockChan:
				blockHeight := block.Data.(tmtypes.EventDataNewBlock).Block.Height
				joltChain.CheckAndUpdatePool(blockHeight)

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
					joltChain.AddMoveFundItem(previousPool, blockHeight)
					pi.addMoveFundItem(previousPool.EthAddress, blockHeight)
				}

				// we move fund if some pool retired
				go func() {
					previousPool, h := joltChain.PopMoveFundItem()
					if previousPool == nil {
						return
					}
					// we get the latest pool address and move funds to the latest pool
					currentPool := pi.GetPool()
					isSigner, err := joltChain.CheckWhetherSigner(previousPool.PoolInfo)
					if err != nil {
						zlog.Logger.Warn().Msg("fail in check whether we are signer in moving fund")
						return
					}
					if !isSigner {
						return
					}
					err = joltChain.MoveFunds(previousPool, currentPool[1].JoltifyAddress, blockHeight)
					if err != nil {
						zlog.Log().Err(err).Msgf("fail to move the fund from %v to %v", previousPool.JoltifyAddress.String(), poolInfo[1].CreatePool.PoolAddr.String())
						joltChain.AddMoveFundItem(previousPool, h)
					}
				}()

				// now we need to put the failed inbound request to the process channel, for each new joltify block
				// we process one failure
				pi.ShowItems()
				itemInbound := pi.PopItem()
				if itemInbound != nil {
					itemInbound.SetItemHeight(blockHeight)
					pi.InboundReqChan <- itemInbound
				}

			case r := <-newJoltifyTxChan:
				blockHeight := r.Data.(tmtypes.EventDataTx).Height
				tx := r.Data.(tmtypes.EventDataTx).Tx
				joltChain.CheckOutBoundTx(blockHeight, tx)

				// process the public chain new block event
			case head := <-pubNewBlockChan:
				err := pi.ProcessNewBlock(head.Number)
				if err != nil {
					zlog.Logger.Error().Err(err).Msg("fail to process the inbound block")
				}
				// we delete the expired tx
				pi.DeleteExpired(head.Number.Uint64())

				// now we need to put the failed outbound request to the process channel
				itemOutBound := joltChain.PopItem()
				if itemOutBound != nil {
					itemOutBound.SetItemHeight(int64(head.Number.Uint64()))
					joltChain.OutboundReqChan <- itemOutBound
				}

			// process the in-bound top up event which will mint coin for users
			case item := <-pi.InboundReqChan:
				// first we check whether this tx has already been submitted by others
				pools := joltChain.GetPool()
				found, err := joltChain.CheckWhetherSigner(pools[1].PoolInfo)
				if err != nil {
					zlog.Logger.Error().Err(err).Msg("fail to check whether we are the node submit the mint request")
					continue
				}
				if found {
					err := joltChain.ProcessInBound(item)
					if err != nil {
						pi.AddItem(item)
						zlog.Logger.Error().Err(err).Msg("fail to mint the coin for the user")
					}

				}

			case item := <-joltChain.OutboundReqChan:
				pools := joltChain.GetPool()
				found, err := joltChain.CheckWhetherSigner(pools[1].PoolInfo)
				if err != nil {
					zlog.Logger.Error().Err(err).Msg("fail to check whether we are the node submit the mint request")
					continue
				}
				if found {
					toAddr, fromAddr, amount, blockHeight := item.GetOutBoundInfo()
					txHash, err := pi.ProcessOutBound(toAddr, fromAddr, amount, blockHeight)
					if err != nil {
						zlog.Logger.Error().Err(err).Msg("fail to broadcast the tx")
						joltChain.AddItem(item)
					}
					zlog.Logger.Info().Msgf(">>we have send outbound tx(%v) from %v to %v (%v)", txHash, fromAddr, toAddr, amount.String())
				}

			}
		}
	}()
}
