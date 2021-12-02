package pubchain

import (
	"errors"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ethereum/go-ethereum/event"
)

const (
	inBoundDenom       = "BNB"
	OutBoundDenom      = "jolt"
	inBoundFeeMin      = "0.000000000000001"
	OUTBoundFeeOut     = "0.000000000000001"
	PendingAccountsize = 1024
	iNBoundToken       = "0x0cD80A18df1C5eAd4B5Fb549391d58B06EFfDBC4"
	iNBoundDenom       = "jusd"
)

const (
	inBound = iota
	outBound
	QueryTimeOut = time.Second * 3
)

// direction is the direction of the joltify_bridge
type direction int

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

// AccountInboundReq is the account that top up account info to joltify pub_chain
type AccountInboundReq struct {
	address     common.Address
	toPoolAddr  common.Address
	coin        sdk.Coin
	blockHeight int64
}

func newAccountInboundReq(address, toPoolAddr common.Address, coin sdk.Coin, blockHeight int64) AccountInboundReq {
	return AccountInboundReq{
		address,
		toPoolAddr,
		coin,
		blockHeight,
	}
}

// GetInboundReqInfo returns the info of the inbound transaction
func (acq *AccountInboundReq) GetInboundReqInfo() (common.Address, common.Address, sdk.Coin, int64) {
	return acq.address, acq.toPoolAddr, acq.coin, acq.blockHeight
}

//SetItemHeight sets the block height of the tx
func (acq *AccountInboundReq) SetItemHeight(blockHeight int64) {
	acq.blockHeight = blockHeight
}

// PubChainInstance hold the joltify_bridge entity
type PubChainInstance struct {
	EthClient             *ethclient.Client
	tokenAddr             string
	logger                zerolog.Logger
	pendingAccounts       map[string]*bridgeTx
	lastTwoPools          []common.Address
	poolLocker            sync.RWMutex
	pendingAccountLocker  sync.RWMutex
	tokenSb               *tokenSb
	AccountInboundReqChan chan *AccountInboundReq
	RetryInboundReq       chan *AccountInboundReq // if a tx fail to process, we need to put in this channel and wait for retry
}

type bridgeTx struct {
	address   common.Address
	direction direction
	timeStamp time.Time
	token     sdk.Coin
	fee       sdk.Coin
}

// NewChainInstance initialize the joltify_bridge entity
func NewChainInstance(ws, tokenAddr string) (*PubChainInstance, error) {
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
		logger:                logger,
		EthClient:             wsClient,
		tokenAddr:             tokenAddr,
		pendingAccounts:       make(map[string]*bridgeTx),
		poolLocker:            sync.RWMutex{},
		pendingAccountLocker:  sync.RWMutex{},
		lastTwoPools:          make([]common.Address, 2),
		tokenSb:               newTokenSb(tokenIns, sink, nil),
		AccountInboundReqChan: make(chan *AccountInboundReq, 512),
		RetryInboundReq:       make(chan *AccountInboundReq, 128),
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
