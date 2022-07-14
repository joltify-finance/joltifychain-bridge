package oppybridge

import (
	"sync"
	"time"

	"gitlab.com/oppy-finance/oppy-bridge/tokenlist"

	"go.uber.org/atomic"

	"github.com/cosmos/cosmos-sdk/simapp/params"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	grpc1 "github.com/gogo/protobuf/grpc"
	bcommon "gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/tssclient"
	"gitlab.com/oppy-finance/oppy-bridge/validators"
	"gitlab.com/oppy-finance/oppychain/x/vault/types"

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
)

// tssPoolMsg this is the pool pre-submit message for the given height
type tssPoolMsg struct {
	msg         *types.MsgCreateCreatePool
	creator     sdk.AccAddress
	poolPubKey  string
	blockHeight int64
}

// OppyChainInstance defines the types for oppy pub_chain side
type OppyChainInstance struct {
	grpcClient       grpc1.ClientConn
	wsClient         *tmclienthttp.HTTP
	encoding         *params.EncodingConfig
	Keyring          keyring.Keyring
	logger           zerolog.Logger
	validatorSet     *validators.ValidatorSet
	myValidatorInfo  info
	tssServer        tssclient.TssInstance
	poolUpdateLocker *sync.RWMutex
	keyGenCache      []tssPoolMsg
	lastTwoPools     []*bcommon.PoolInfo
	OutboundReqChan  chan *bcommon.OutBoundReq
	RetryOutboundReq *sync.Map // if a tx fail to process, we need to put in this channel and wait for retry
	moveFundReq      *sync.Map
	CurrentHeight    int64
	inBoundGas       *atomic.Int64
	outBoundGasPrice *atomic.Int64
	TokenList        tokenlist.TokenListI
	pendingTx        *sync.Map
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

type OutboundTx struct {
	OutReceiverAddress common.Address `json:"receiver_address"`
	BlockHeight        uint64         `json:"block_height"`
	Token              sdk.Coin       `json:"token"`
	TokenAddr          string         `json:"token_addr"`
	Fee                sdk.Coin       `json:"fee"`
	TxID               string         `json:"txid"`
}
