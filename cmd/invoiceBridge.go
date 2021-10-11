package main

import (
	"context"
	"fmt"
	"github.com/cosmos/cosmos-sdk/types"
	golog "github.com/ipfs/go-log"
	"github.com/joltgeorge/tss/common"
	tmtypes "github.com/tendermint/tendermint/types"
	"invoicebridge/bridge"
	"invoicebridge/config"
	"log"
	"os"
	"os/signal"
	"path"
	"sync"
	"time"
)

func SetupBech32Prefix() {
	config := types.GetConfig()
	// thorchain will import go-tss as a library , thus this is not needed, we copy the prefix here to avoid go-tss to import thorchain
	config.SetBech32PrefixForAccount("inv", "invpub")
	config.SetBech32PrefixForValidator("invval", "invvpub")
	config.SetBech32PrefixForConsensusNode("invvalcons", "invcpub")
}

func main() {
	SetupBech32Prefix()
	config := config.DefaultConfig()

	_ = golog.SetLogLevel("tss-lib", "INFO")
	common.InitLog("info", true, "invoiceBridge_service")

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
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	err = invBridge.InitValidators(config.InvoiceChainConfig.RPCAddress)
	if err != nil {
		fmt.Printf("error in init the validators %v", err)
		return
	}
	tssHttpServer := NewTssHttpServer(config.TssConfig.HttpAddr, invBridge.GetTssNodeID(), ctx)

	wg.Add(1)
	ret := tssHttpServer.Start(&wg)
	if ret != nil {
		cancel()
		return
	}

	wg.Add(1)
	addEventLoop(ctx, wg, invBridge)

	select {
	case <-c:
		cancel()
		return
	case <-ctx.Done():
		return
	}
	wg.Wait()
	fmt.Printf("we quit gracefully\n")
}

func addEventLoop(ctx context.Context, wg sync.WaitGroup, invBridge *bridge.InvChainBridge) {

	defer wg.Done()
	query := "tm.event = 'ValidatorSetUpdates'"
	ctxLocal, cancelLocal := context.WithTimeout(ctx, time.Second*5)
	defer cancelLocal()
	outChan, err := invBridge.AddSubscribe(ctxLocal, query)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return
	}

	go func() {

		for {
			select {
			case <-ctx.Done():
				return
			case vals := <-outChan:
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
			}
		}

	}()

}
