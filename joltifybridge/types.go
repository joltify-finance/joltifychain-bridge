package joltifybridge

import (
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
	"gitlab.com/joltify/joltifychain-bridge/validators"
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

//tssPoolMsg this is the pool pre-submit message for the given height
type tssPoolMsg struct {
	data        []byte
	poolPubKey  string
	blockHeight int64
}

//JoltifyChainBridge defines the types for joltify pub_chain side
type JoltifyChainBridge struct {
	grpcClient        *grpc.ClientConn
	wsClient          *tmclienthttp.HTTP
	keyring           keyring.Keyring
	logger            zerolog.Logger
	validatorSet      *validators.ValidatorSet
	myValidatorInfo   info
	tssServer         *tssclient.BridgeTssServer
	cosKey            tssclient.CosPrivKey
	poolUpdateLocker  *sync.Mutex
	msgSendCache      []tssPoolMsg
	LastTwoTssPoolMsg []*tssPoolMsg
}

//info the import structure of the cosmos validator info
type info struct {
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
