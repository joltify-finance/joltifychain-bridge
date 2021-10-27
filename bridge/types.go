package bridge

import (
	"joltifybridge/tssclient"
	"joltifybridge/validators"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/rs/zerolog"
	tmclienthttp "github.com/tendermint/tendermint/rpc/client/http"
	"google.golang.org/grpc"
)

const (
	grpcTimeout = time.Second * 10
	chainID     = "joltifyChain"
)

type TssPoolMsg struct {
	data        []byte
	blockHeight int64
}

type InvChainBridge struct {
	grpcClient        *grpc.ClientConn
	wsClient          *tmclienthttp.HTTP
	keyring           keyring.Keyring
	logger            zerolog.Logger
	validatorSet      *validators.ValidatorSet
	myValidatorInfo   Info
	tssServer         *tssclient.BridgeTssServer
	cosKey            tssclient.CosPrivKey
	poolUpdateLocker  *sync.Mutex
	msgSendCache      []TssPoolMsg
	LastTwoTssPoolMsg []*TssPoolMsg
}

type Info struct {
	Result struct {
		ValidatorInfo struct {
			Address string `json:"address"`
			PubKey  struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"pub_key"`
			VotingPower string `json:"voting_power"`
		} `json:"validator_info"`
	} `json:"result"`
}
