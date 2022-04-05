package joltifybridge

import (
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/cosmos/cosmos-sdk/simapp/params"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ethereum/go-ethereum/common"
	grpc1 "github.com/gogo/protobuf/grpc"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
	"gitlab.com/joltify/joltifychain-bridge/validators"
	"gitlab.com/joltify/joltifychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/rs/zerolog"
	tmclienthttp "github.com/tendermint/tendermint/rpc/client/http"
)

const (
	grpcTimeout   = time.Second * 30
	reqCacheSize  = 1024
	ROUNDBLOCK    = 50
	submitBackoff = time.Millisecond * 500
	GroupBlockGap = 6
	GroupSign     = 8
)

var chainID = "joltifyChain"

// tssPoolMsg this is the pool pre-submit message for the given height
type tssPoolMsg struct {
	msg         *types.MsgCreateCreatePool
	acc         authtypes.AccountI
	poolPubKey  string
	blockHeight int64
}

// JoltifyChainInstance defines the types for joltify pub_chain side
type JoltifyChainInstance struct {
	grpcClient       grpc1.ClientConn
	wsClient         *tmclienthttp.HTTP
	encoding         *params.EncodingConfig
	Keyring          keyring.Keyring
	logger           zerolog.Logger
	validatorSet     *validators.ValidatorSet
	myValidatorInfo  info
	tssServer        tssclient.TssInstance
	poolUpdateLocker *sync.RWMutex
	msgSendCache     []tssPoolMsg
	lastTwoPools     []*bcommon.PoolInfo
	OutboundReqChan  chan *bcommon.OutBoundReq
	RetryOutboundReq *sync.Map // if a tx fail to process, we need to put in this channel and wait for retry
	moveFundReq      *sync.Map
	CurrentHeight    int64
	inboundGas       *atomic.Int64
}

// info the import structure of the cosmos validator info
type info struct {
	Result struct {
		ValidatorInfo struct {
			Address string `json:"outReceiverAddress"`
			PubKey  struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"pub_key"`
			VotingPower string `json:"voting_power"`
		} `json:"validator_info"`
	} `json:"result"`
}

type outboundTx struct {
	outReceiverAddress common.Address
	blockHeight        uint64
	token              sdk.Coin
	fee                sdk.Coin
}
