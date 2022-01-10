package pubchain

import (
	"errors"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ethereum/go-ethereum/event"

	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
)

const (
	reqCacheSize      = 512
	retryCacheSize    = 128
	chainQueryTimeout = time.Second * 5
	GasLimit          = 2100000
	GasPrice          = "0.00000001"
)

type tokenSb struct {
	tokenInstance *Token
	sb            chan *TokenTransfer
	sbEvent       event.Subscription
	lock          *sync.RWMutex
}

// UpdateSbEvent at the start of the subscription
func (tk *tokenSb) UpdateSbEvent(sbEvent event.Subscription) {
	tk.lock.Lock()
	defer tk.lock.Unlock()
	// we unsubscribe the previous event
	if tk.sbEvent != nil {
		tk.sbEvent.Unsubscribe()
	}
	tk.sbEvent = sbEvent
}

// InboundReq is the account that top up account info to joltify pub_chain
type InboundReq struct {
	address     sdk.AccAddress
	toPoolAddr  common.Address
	coin        sdk.Coin
	blockHeight int64
}

func newAccountInboundReq(address sdk.AccAddress, toPoolAddr common.Address, coin sdk.Coin, blockHeight int64) InboundReq {
	return InboundReq{
		address,
		toPoolAddr,
		coin,
		blockHeight,
	}
}

// GetInboundReqInfo returns the info of the inbound transaction
func (acq *InboundReq) GetInboundReqInfo() (sdk.AccAddress, common.Address, sdk.Coin, int64) {
	return acq.address, acq.toPoolAddr, acq.coin, acq.blockHeight
}

// SetItemHeight sets the block height of the tx
func (acq *InboundReq) SetItemHeight(blockHeight int64) {
	acq.blockHeight = blockHeight
}

// PubChainInstance hold the joltify_bridge entity
type PubChainInstance struct {
	EthClient              *ethclient.Client
	tokenAddr              string
	logger                 zerolog.Logger
	pendingInbounds        map[string]*inboundTx
	lastTwoPools           []*bcommon.PoolInfo
	poolLocker             *sync.RWMutex
	pendingInboundTxLocker *sync.RWMutex
	tokenSb                *tokenSb
	tssServer              *tssclient.BridgeTssServer
	InboundReqChan         chan *InboundReq
	RetryInboundReq        chan *InboundReq // if a tx fail to process, we need to put in this channel and wait for retry
}

type inboundTx struct {
	address     sdk.AccAddress
	blockHeight uint64
	token       sdk.Coin
	fee         sdk.Coin
}

// NewChainInstance initialize the joltify_bridge entity
func NewChainInstance(ws, tokenAddr string, tssServer *tssclient.BridgeTssServer) (*PubChainInstance, error) {
	logger := log.With().Str("module", "pubchain").Logger()

	wsClient, err := ethclient.Dial(ws)
	if err != nil {
		logger.Error().Err(err).Msg("fail to dial the websocket")
		return nil, errors.New("fail to dial the network")
	}

	sink := make(chan *TokenTransfer)

	tokenIns, err := NewToken(common.HexToAddress(tokenAddr), wsClient)
	if err != nil {
		return nil, errors.New("fail to get the new token")
	}

	return &PubChainInstance{
		logger:                 logger,
		EthClient:              wsClient,
		tokenAddr:              tokenAddr,
		pendingInbounds:        make(map[string]*inboundTx),
		poolLocker:             &sync.RWMutex{},
		pendingInboundTxLocker: &sync.RWMutex{},
		tssServer:              tssServer,
		lastTwoPools:           make([]*bcommon.PoolInfo, 2),
		tokenSb:                newTokenSb(tokenIns, sink, nil),
		InboundReqChan:         make(chan *InboundReq, reqCacheSize),
		RetryInboundReq:        make(chan *InboundReq, retryCacheSize),
	}, nil
}

// newTokenSb create the token instance
func newTokenSb(instance *Token, sb chan *TokenTransfer, sbEvent event.Subscription) *tokenSb {
	return &tokenSb{
		instance,
		sb,
		sbEvent,
		&sync.RWMutex{},
	}
}
