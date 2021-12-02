package bridge

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"sync"
	"time"

	"gitlab.com/joltify/joltifychain/joltifychain-bridge/misc"

	"gitlab.com/joltify/joltifychain/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/joltifybridge"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/pubchain"

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

	keyringPath := path.Join(config.HomeDir, config.KeyringAddress)
	joltifyBridge, err := joltifybridge.NewJoltifyBridge(config.InvoiceChainConfig.GrpcAddress, keyringPath, "12345678", config)
	if err != nil {
		log.Fatalln("fail to create the invoice joltify_bridge", err)
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
	ci, err := pubchain.NewChainInstance(config.PubChainConfig.WsAddress, config.PubChainConfig.TokenAddress)
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

func addEventLoop(ctx context.Context, wg *sync.WaitGroup, joltBridge *joltifybridge.JoltifyChainBridge, ci *pubchain.PubChainInstance) {
	defer wg.Done()
	query := "tm.event = 'ValidatorSetUpdates'"
	ctxLocal, cancelLocal := context.WithTimeout(ctx, time.Second*5)
	defer cancelLocal()

	validatorUpdateChan, err := joltBridge.AddSubscribe(ctxLocal, query)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return
	}

	query = "tm.event = 'NewBlock'"
	newBlockChan, err := joltBridge.AddSubscribe(ctxLocal, query)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return
	}

	wg.Add(1)

	// pubNewBlockChan is the channel for the new blocks for the public chain
	pubNewBlockChan, err := ci.StartSubscription(ctx, wg)
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
				height, err := joltBridge.GetLastBlockHeight()
				if err != nil {
					continue
				}
				validatorUpdates := vals.Data.(tmtypes.EventDataValidatorSetUpdates).ValidatorUpdates
				err = joltBridge.HandleUpdateValidators(validatorUpdates, height)
				if err != nil {
					fmt.Printf("error in handle update validator")
					continue
				}

				// process the new joltify block, validator may need to submit the pool address
			case block := <-newBlockChan:
				updated, poolPubKey := joltBridge.CheckAndUpdatePool(block.Data.(tmtypes.EventDataNewBlock).Block.Height)
				if updated {
					// var poolPubkey []
					addr, err := misc.PoolPubKeyToEthAddress(poolPubKey)
					if err != nil {
						fmt.Printf("fail to convert the jolt address to eth address %v", poolPubKey)
						continue
					}
					ci.UpdatePool(addr)

					fmt.Printf("update the address %v\n", ci.GetPool())
					err = ci.UpdateSubscribe()
					if err != nil {
						zlog.Logger.Error().Err(err).Msg("fail to subscribe the new transfer pool address")
					}
				}

				// process the public chain inbound message to the channel
			case tokenTransfer := <-ci.GetSubChannel():
				err := ci.ProcessInBound(tokenTransfer)
				if err != nil {
					zlog.Logger.Error().Err(err).Msg("fail to process the inbound contract message")
				}

				// process the public chain new block event
			case head := <-pubNewBlockChan:
				err := ci.ProcessNewBlock(head.Number)
				if err != nil {
					zlog.Logger.Error().Err(err).Msg("fail to process the inbound block")
				}
				// now we need to put the failed inbound request to the process channel, for each new joltify block
				// we process one failure
				select {
				case item := <-ci.RetryInboundReq:
					item.SetItemHeight(head.Number.Int64())
					ci.AccountInboundReqChan <- item
					continue
				default:
					continue
				}

			// process the in-bound top up event which will mint coin for users
			case item := <-ci.AccountInboundReqChan:
				// first we check whether this tx has already been submitted by others
				found, err := joltBridge.CheckWhetherSigner()
				if err != nil {
					zlog.Logger.Error().Err(err).Msg("fail to check whether we are the node submit the mint request")
					continue
				}

				if found {
					err := joltBridge.MintCoin(item)
					if err != nil {
						ci.RetryInboundReq <- item
						zlog.Logger.Error().Err(err).Msg("fail to mint the coin for the user")
					}
				}
			}
		}
	}()
}
