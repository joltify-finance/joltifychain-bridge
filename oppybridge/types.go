package oppybridge

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	ctypes "github.com/tendermint/tendermint/rpc/core/types"
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
	channelSize   = 2000
	ROUNDBLOCK    = 20
	submitBackoff = time.Millisecond * 500
	GroupBlockGap = 4
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
	grpcAddr              string
	httpAddr              string
	grpcLock              *sync.RWMutex
	GrpcClient            grpc1.ClientConn
	WsClient              *tmclienthttp.HTTP
	encoding              *params.EncodingConfig
	Keyring               keyring.Keyring
	logger                zerolog.Logger
	validatorSet          *validators.ValidatorSet
	myValidatorInfo       info
	tssServer             tssclient.TssInstance
	poolUpdateLocker      *sync.RWMutex
	keyGenCache           []tssPoolMsg
	lastTwoPools          []*bcommon.PoolInfo
	OutboundReqChan       chan []*bcommon.OutBoundReq
	RetryOutboundReq      *sync.Map // if a tx fail to process, we need to put in this channel and wait for retry
	CurrentHeight         int64
	inBoundGas            *atomic.Int64
	outBoundFee           *atomic.Int64
	TokenList             tokenlist.BridgeTokenListI
	pendingTx             *sync.Map
	ChannelQueueNewBlock  chan ctypes.ResultEvent
	ChannelQueueValidator chan ctypes.ResultEvent
	CurrentNewBlockChan   <-chan ctypes.ResultEvent
	CurrentNewValidator   <-chan ctypes.ResultEvent
	retryLock             *sync.Mutex
	onHoldRetryQueueLock  *sync.Mutex
	onHoldRetryQueue      []*bcommon.OutBoundReq
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
	FromAddress        string         `json:"from_address"` // this item is used in query pending to to match a given sender
	ChainID            string         `json:"chain_id"`
	BlockHeight        uint64         `json:"block_height"`
	Token              sdk.Coin       `json:"token"`
	TokenAddr          string         `json:"token_addr"`
	Fee                sdk.Coin       `json:"fee"`
	FeeWanted          sdk.Coin       `json:"fee_wanted"`
	TxID               string         `json:"txid"`
}

// NewOppyBridge new the instance for the oppy pub_chain
func NewOppyBridge(grpcAddr, httpAddr string, tssServer tssclient.TssInstance, tl tokenlist.BridgeTokenListI) (*OppyChainInstance, error) {
	var oppyBridge OppyChainInstance
	var err error
	oppyBridge.logger = log.With().Str("module", "oppyChain").Logger()
	oppyBridge.GrpcClient, err = grpc.Dial(grpcAddr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	client, err := tmclienthttp.New(httpAddr, "/websocket")
	if err != nil {
		return nil, err
	}
	err = client.Start()
	if err != nil {
		return nil, err
	}

	oppyBridge.WsClient = client

	oppyBridge.Keyring = keyring.NewInMemory()

	oppyBridge.tssServer = tssServer

	oppyBridge.keyGenCache = []tssPoolMsg{}
	oppyBridge.lastTwoPools = make([]*bcommon.PoolInfo, 2)
	oppyBridge.poolUpdateLocker = &sync.RWMutex{}
	oppyBridge.inBoundGas = atomic.NewInt64(250000)
	oppyBridge.outBoundFee = atomic.NewInt64(5000000000)

	encode := MakeEncodingConfig()
	oppyBridge.encoding = &encode
	oppyBridge.OutboundReqChan = make(chan []*bcommon.OutBoundReq, reqCacheSize)
	oppyBridge.RetryOutboundReq = &sync.Map{}
	oppyBridge.TokenList = tl
	oppyBridge.pendingTx = &sync.Map{}
	oppyBridge.ChannelQueueNewBlock = make(chan ctypes.ResultEvent, channelSize)
	oppyBridge.ChannelQueueValidator = make(chan ctypes.ResultEvent, channelSize)
	oppyBridge.grpcLock = &sync.RWMutex{}
	oppyBridge.grpcAddr = grpcAddr
	oppyBridge.httpAddr = httpAddr
	oppyBridge.retryLock = &sync.Mutex{}
	oppyBridge.onHoldRetryQueueLock = &sync.Mutex{}
	oppyBridge.onHoldRetryQueue = []*bcommon.OutBoundReq{}
	return &oppyBridge, nil
}
