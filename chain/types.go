package chain

import (
	"errors"
	"sync"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ethereum/go-ethereum/event"
)

const (
	inBoundDenom       = "BNB"
	OutBoundDenom      = "jolt"
	inBoundFeeMin      = "0.02"
	OUTBoundFeeOut     = "0.02"
	PendingAccountsize = 1024
	iNBoundToken       = "0x6CA715403f18259e971cbfe74aEe60beA3781bA6"
)

const (
	inBound = iota
	outBound
)

//direction is the direction of the bridge
type direction int

type tokenSb struct {
	tokenInstance *Token
	sb            chan *TokenTransfer
	sbEvent       *event.Subscription
	lock          *sync.RWMutex
}

//PubChainInstance hold the bridge entity
type PubChainInstance struct {
	EthClient            *ethclient.Client
	tokenAddr            string
	logger               zerolog.Logger
	pendingAccounts      map[string]*bridgeTx
	lastTwoPools         []string
	poolLocker           sync.RWMutex
	pendingAccountLocker sync.RWMutex
	tokenSb              *tokenSb
}

type bridgeTx struct {
	address   string
	direction direction
	timeStamp time.Time
	token     sdk.Coin
	fee       sdk.Coin
}

//NewChainInstance initialize the bridge entity
func NewChainInstance(ws, tokenAddr string) (*PubChainInstance, error) {
	logger := log.With().Str("module", "pubchain").Logger()

	wsClient, err := ethclient.Dial(ws)
	if err != nil {
		logger.Error().Err(err).Msg("fail to dial the websocket")
		return nil, errors.New("fail to dial the network")
	}

	sink := make(chan *TokenTransfer)

	return &PubChainInstance{
		logger:               logger,
		EthClient:            wsClient,
		tokenAddr:            tokenAddr,
		pendingAccounts:      make(map[string]*bridgeTx),
		poolLocker:           sync.RWMutex{},
		pendingAccountLocker: sync.RWMutex{},
		lastTwoPools:         make([]string, 2),
		tokenSb:              NewTokenSb(nil, sink, nil),
	}, nil
}

//NewTokenSb create the token instance
func NewTokenSb(instance *Token, sb chan *TokenTransfer, sbEvent *event.Subscription) *tokenSb {
	return &tokenSb{
		instance,
		sb,
		sbEvent,
		&sync.RWMutex{},
	}
}
