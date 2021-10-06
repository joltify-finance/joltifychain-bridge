package bridge

import (
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

const chainID = "invoiceChain"

type InvChainBridge struct {
	grpcClient *grpc.ClientConn
	keyring    keyring.Keyring
	logger     zerolog.Logger
}
