package main

import (
	"invoicebridge/bridge"
	"invoicebridge/config"
	"log"
	"os"
	"path"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/cosmos-sdk/types"
)

func SetupBech32Prefix() {
	config := types.GetConfig()
	// thorchain will import go-tss as a library , thus this is not needed, we copy the prefix here to avoid go-tss to import thorchain
	config.SetBech32PrefixForAccount("inv", "invpub")
	config.SetBech32PrefixForValidator("invvv", "invvpub")
	config.SetBech32PrefixForConsensusNode("invc", "invcpub")
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
	invBridge, err := bridge.NewInvoiceBridge(config.InvoiceChainConfig.GrpcAddress, keyringPath, "12345678")
	if err != nil {
		log.Fatalln("fail to create the invoice bridge", err)
		return
	}
	from, err := sdk.AccAddressFromBech32("inv1rfmwldwrm3652shx3a7say0v4vvtglass0kv58")
	if err != nil {
		panic(err)
	}

	to, err := sdk.AccAddressFromBech32("inv1xdpg5l3pxpyhxqg4ey4krq2pf9d3sphmqu4lgz")
	if err != nil {
		panic(err)
	}

	coin := sdk.Coin{
		Denom:  "VUSD",
		Amount: sdk.NewIntFromUint64(1000),
	}
	err = invBridge.SendToken(sdk.Coins{coin}, from, to)
	if err != nil {
		panic(err)
	}
}
