package cosbridge

import (
	"sync"
	"time"

	"github.com/gogo/protobuf/grpc"

	cossubmit "gitlab.com/joltify/joltifychain-bridge/cos_submit"
	"gitlab.com/joltify/joltifychain-bridge/tokenlist"

	"github.com/cosmos/cosmos-sdk/simapp/params"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/joltify-finance/joltify_lending/x/vault/types"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
	"gitlab.com/joltify/joltifychain-bridge/validators"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/rs/zerolog"
)

const (
	grpcTimeout   = time.Second * 30
	reqCacheSize  = 1024
	ROUNDBLOCK    = 20
	GroupBlockGap = 4
)

// tssPoolMsg this is the pool pre-submit message for the given height
type tssPoolMsg struct {
	msg         *types.MsgCreateCreatePool
	creator     sdk.AccAddress
	poolPubKey  string
	blockHeight int64
}

// JoltChainInstance defines the types for joltify pub_chain side
type JoltChainInstance struct {
	encoding             *params.EncodingConfig
	logger               zerolog.Logger
	validatorSet         *validators.ValidatorSet
	myValidatorInfo      info
	poolUpdateLocker     *sync.RWMutex
	keyGenCache          []tssPoolMsg
	lastTwoPools         []*bcommon.PoolInfo
	OutboundReqChan      chan []*bcommon.OutBoundReq
	RetryOutboundReq     *sync.Map // if a tx fail to process, we need to put in this channel and wait for retry
	CurrentHeight        int64
	TokenList            tokenlist.BridgeTokenListI
	onHoldRetryQueueLock *sync.Mutex
	onHoldRetryQueue     []*bcommon.OutBoundReq
	FeeModule            map[string]*bcommon.FeeModule
	CosHandler           *cossubmit.CosHandler
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
	OutReceiverAddress string   `json:"receiver_address"`
	FromAddress        string   `json:"from_address"` // this item is used in query pending to to match a given sender
	BlockHeight        uint64   `json:"block_height"`
	Token              sdk.Coin `json:"token"`
	TokenAddr          string   `json:"token_addr"`
	Fee                sdk.Coin `json:"fee"`
	TxID               string   `json:"txid"`
	ChainType          string   `json:"chain_type"`
}

// NewJoltifyBridge new the instance for the joltify pub_chain
func NewJoltifyBridge(grpcAddr, httpAddr string, grpcClient grpc.ClientConn, tssServer tssclient.TssInstance, tl tokenlist.BridgeTokenListI, retryPools *bcommon.RetryPools) (*JoltChainInstance, error) {
	var joltBridge JoltChainInstance
	var err error

	key := keyring.NewInMemory()

	handler, err := cossubmit.NewCosOperations(grpcAddr, httpAddr, grpcClient, key, "joltify", tssServer)
	if err != nil {
		return nil, err
	}
	joltBridge.CosHandler = handler

	joltBridge.keyGenCache = []tssPoolMsg{}
	joltBridge.lastTwoPools = make([]*bcommon.PoolInfo, 2)
	joltBridge.poolUpdateLocker = &sync.RWMutex{}

	encode := bcommon.MakeEncodingConfig()
	joltBridge.encoding = &encode
	joltBridge.OutboundReqChan = make(chan []*bcommon.OutBoundReq, reqCacheSize)
	joltBridge.RetryOutboundReq = retryPools.RetryOutboundReq
	joltBridge.TokenList = tl
	joltBridge.onHoldRetryQueueLock = &sync.Mutex{}
	joltBridge.onHoldRetryQueue = []*bcommon.OutBoundReq{}
	joltBridge.FeeModule = make(map[string]*bcommon.FeeModule)
	// we set the bridge fee
	joltBridge.FeeModule = bcommon.InitFeeModule()
	return &joltBridge, nil
}
