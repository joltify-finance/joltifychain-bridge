package bridge
import (
	"google.golang.org/grpc"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
)

type InvChainBridge struct{
	GrpcClient *grpc.ClientConn
	Keyring    keyring.Keyring
}
