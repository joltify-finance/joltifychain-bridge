package main

import (
	"fmt"
	"invoicebridge/bridge"
	"invoicebridge/config"
	"log"
	"os"
	//"os"
)

func main() {
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

	invBridge, err := bridge.NewInvoiceBridge(config.HomeDir, config.KeyringAddress, "12345678")
	if err != nil {
		log.Fatalln("fail to create the invoice bridge", err)
		return
	}
	valkey, err := invBridge.Keyring.Key("operator")
	if err != nil {

		log.Fatalln("fail to create the invoice bridge", err)
		return
	}
	fmt.Printf("----->%v\n", valkey.GetName())

}
