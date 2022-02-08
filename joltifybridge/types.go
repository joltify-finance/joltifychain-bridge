package joltifybridge

import (
	"sync"
	"time"

	grpc1 "github.com/gogo/protobuf/grpc"

	"github.com/cosmos/cosmos-sdk/simapp/params"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ethereum/go-ethereum/common"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
	"gitlab.com/joltify/joltifychain-bridge/validators"
	"gitlab.com/joltify/joltifychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/rs/zerolog"
	tmclienthttp "github.com/tendermint/tendermint/rpc/client/http"
)

const (
	grpcTimeout  = time.Second * 10
	chainID      = "joltifyChain"
	reqCacheSize = 512
)

// tssPoolMsg this is the pool pre-submit message for the given height
type tssPoolMsg struct {
	msg         *types.MsgCreateCreatePool
	acc         authtypes.AccountI
	poolPubKey  string
	blockHeight int64
}

// JoltifyChainBridge defines the types for joltify pub_chain side
type JoltifyChainBridge struct {
	grpcClient       grpc1.ClientConn
	wsClient         *tmclienthttp.HTTP
	encoding         *params.EncodingConfig
	Keyring          keyring.Keyring
	logger           zerolog.Logger
	validatorSet     *validators.ValidatorSet
	myValidatorInfo  info
	tssServer        tssclient.TssSign
	poolUpdateLocker *sync.RWMutex
	msgSendCache     []tssPoolMsg
	lastTwoPools     []*bcommon.PoolInfo
	OutboundReqChan  chan *OutBoundReq
	TransferChan     []*<-chan ctypes.ResultEvent
	RetryOutboundReq chan *OutBoundReq // if a tx fail to process, we need to put in this channel and wait for retry
	poolAccInfo      *poolAccInfo
	poolAccLocker    *sync.Mutex
}

type poolAccInfo struct {
	accountNum uint64
	accSeq     uint64
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

// OutBoundReq is the entity for the outbound tx
type OutBoundReq struct {
	outReceiverAddress common.Address
	fromPoolAddr       common.Address
	coin               sdk.Coin
	blockHeight        int64
}

func newAccountOutboundReq(address, fromPoolAddr common.Address, coin sdk.Coin, blockHeight int64) OutBoundReq {
	return OutBoundReq{
		address,
		fromPoolAddr,
		coin,
		blockHeight,
	}
}
