package main

import (
	"context"
	"fmt"
	"invoicebridge/bridge"
	"invoicebridge/config"
	"log"
	"os"
	"os/signal"
	"path"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/types"
	tmtypes "github.com/tendermint/tendermint/types"
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	wg.Add(1)

	query := "tm.event = 'ValidatorSetUpdates'"
	outChan, err := invBridge.AddSubscribe(ctx, query)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return
	}

	err = invBridge.InitValidators(config.InvoiceChainConfig.RPCAddress)
	if err != nil {
		fmt.Printf("error in init the validators %v", err)
		return
	}
	go func() {
		defer wg.Done()
		for {
			select {
			case <-c:
				cancel()
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

	wg.Wait()
	fmt.Printf("we quit gracefully\n")
}
