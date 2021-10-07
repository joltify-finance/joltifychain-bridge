package bridge

import (
	"invoicebridge/validators"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/rs/zerolog"
	tmclienthttp "github.com/tendermint/tendermint/rpc/client/http"
	"google.golang.org/grpc"
)

const (
	grpcTimeout = time.Second * 10
	chainID     = "invoiceChain"
)

type InvChainBridge struct {
	grpcClient   *grpc.ClientConn
	wsClient     *tmclienthttp.HTTP
	keyring      keyring.Keyring
	logger       zerolog.Logger
	validatorSet *validators.ValidatorSet
}
