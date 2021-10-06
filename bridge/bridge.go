package bridge

import (
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"google.golang.org/grpc"
	"io/ioutil"
	"log"
)

func NewInvoiceBridge(grpcAddr, keyringPath, passcode string) (*InvChainBridge, error) {
	var invoiceBridge InvChainBridge
	var err error
	invoiceBridge.GrpcClient, err = grpc.Dial(grpcAddr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	invoiceBridge.Keyring = keyring.NewInMemory()

	dat, err := ioutil.ReadFile(keyringPath)
	if err != nil {
		log.Fatalln("error in read keyring file")
		return nil, err
	}
	err = invoiceBridge.Keyring.ImportPrivKey("operator", string(dat), passcode)
	if err != nil {
		return nil, err
	}
	return &invoiceBridge, nil
}
