package main

import (
	"context"
	"fmt"
	"joltifybridge/bridge"
	"joltifybridge/chain"
	"joltifybridge/config"
	"log"
	"os"
	"os/signal"
	"path"
	"sync"
	"time"

	"github.com/rs/zerolog"

	zlog "github.com/rs/zerolog/log"

	"github.com/cosmos/cosmos-sdk/types"

	golog "github.com/ipfs/go-log"
	"github.com/joltgeorge/tss/common"
	tmtypes "github.com/tendermint/tendermint/types"
)

func setupBech32Prefix() {
	config := types.GetConfig()
	// thorchain will import go-tss as a library , thus this is not needed, we copy the prefix here to avoid go-tss to import thorchain
	config.SetBech32PrefixForAccount("jolt", "joltpub")
	config.SetBech32PrefixForValidator("joltval", "joltvpub")
	config.SetBech32PrefixForConsensusNode("joltvalcons", "joltcpub")
}

func main() {
	setupBech32Prefix()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	config := config.DefaultConfig()

	_ = golog.SetLogLevel("tss-lib", "INFO")
	common.InitLog("info", true, "joltifyBridge_service")

	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
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
	invBridge, err := bridge.NewInvoiceBridge(config.InvoiceChainConfig.GrpcAddress, keyringPath, "12345678", config)
	if err != nil {
		log.Fatalln("fail to create the invoice bridge", err)
		return
	}
	defer func() {
		err := invBridge.TerminateBridge()
		if err != nil {
			return
		}
	}()

	err = invBridge.InitValidators(config.InvoiceChainConfig.RPCAddress)
	if err != nil {
		fmt.Printf("error in init the validators %v", err)
		return
	}
	tssHTTPServer := NewTssHttpServer(config.TssConfig.HttpAddr, invBridge.GetTssNodeID(), ctx)

	wg.Add(1)
	ret := tssHTTPServer.Start(&wg)
	if ret != nil {
		cancel()
		return
	}

	// now we monitor the bsc transfer event
	ci, err := chain.NewChainInstance(config.PubChainConfig.WsAddress, config.PubChainConfig.TokenAddress)
	if err != nil {
		fmt.Printf("fail to connect the public chain with address %v\n", config.PubChainConfig.WsAddress)
		return
	}
	//fixme for current testing purpose,we set the pool address to be the local wallet
	err = ci.UpdatePool("bDf7Fb0Ad9b0D722ea54D808b79751608E7AE991")
	if err != nil {
		fmt.Printf("invalid pool address!!!")
		return
	}

	wg.Add(1)
	addEventLoop(ctx, &wg, invBridge, ci)

	select {
	case <-c:
		ctx.Done()
		cancel()
	}
	wg.Wait()
	fmt.Printf("we quit gracefully\n")
}

func addEventLoop(ctx context.Context, wg *sync.WaitGroup, invBridge *bridge.InvChainBridge, ci *chain.PubChainInstance) {
	defer wg.Done()
	query := "tm.event = 'ValidatorSetUpdates'"
	ctxLocal, cancelLocal := context.WithTimeout(ctx, time.Second*5)
	defer cancelLocal()
	subOutChan, err := invBridge.AddSubscribe(ctxLocal, query)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return
	}

	query = "tm.event = 'NewBlock'"
	outChanNewBlock, err := invBridge.AddSubscribe(ctxLocal, query)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return
	}

	wg.Add(1)

	blockHeadChan, err := ci.StartSubscription(ctx, wg)
	if err != nil {
		fmt.Printf("fail to subscribe the token transfer with err %v\n", err)
		return
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case vals := <-subOutChan:
				height, err := invBridge.GetLastBlockHeight()
				if err != nil {
					continue
				}
				validatorUpdates := vals.Data.(tmtypes.EventDataValidatorSetUpdates).ValidatorUpdates
				err = invBridge.HandleUpdateValidators(validatorUpdates, height)
				if err != nil {
					fmt.Printf("error in handle update validator")
					continue
				}
			case block := <-outChanNewBlock:
				invBridge.TriggerSend(block.Data.(tmtypes.EventDataNewBlock).Block.Height)
			case tokenTransfer := <-ci.GetSubChannel():
				err := ci.ProcessInBound(tokenTransfer)
				if err != nil {
					zlog.Logger.Error().Err(err).Msg("fail to process the inbound contract message")
				}
			case head := <-blockHeadChan:
				err := ci.ProcessNewBlock(head.Number)
				if err != nil {
					zlog.Logger.Error().Err(err).Msg("fail to process the inbound block")
				}
			}
		}
	}()
}
