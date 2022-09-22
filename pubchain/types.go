package pubchain

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/oppy-finance/oppy-bridge/tokenlist"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/rs/zerolog"
	"gitlab.com/oppy-finance/oppy-bridge/generated"
	"gitlab.com/oppy-finance/oppy-bridge/tssclient"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog/log"

	"github.com/ethereum/go-ethereum/core/types"
	bcommon "gitlab.com/oppy-finance/oppy-bridge/common"
)

const (
	inboundprosSize   = 512
	sbchannelsize     = 20000
	chainQueryTimeout = time.Second * 15
	ROUNDBLOCK        = 50
	submitBackoff     = time.Second * 4
	GroupBlockGap     = 8
	GroupSign         = 4
	PRICEUPDATEGAP    = 10
	movefundretrygap  = 3
)

var OppyContractAddress = "0x94277968dff216265313657425d9d7577ad32dd1"

type InboundTx struct {
	TxID           string         `json:"tx_id"` // this variable is used for locally saving and loading
	Address        sdk.AccAddress `json:"address"`
	PubBlockHeight uint64         `json:"pub_block_height"` // this variable is used to delete the expired tx
	Token          sdk.Coin       `json:"token"`
}

type Erc20TxInfo struct {
	fromAddr     sdk.AccAddress // address that we should send token to
	toAddr       common.Address // address to the pool
	Amount       *big.Int       // the amount of the token that cross the bridge
	tokenAddress common.Address // the erc20 contract address (we need to ensure the token address is 100% correct)
}

type InboundTxBnb struct {
	BlockHeight uint64   `json:"block_height"`
	TxID        string   `json:"tx_id"`
	Fee         sdk.Coin `json:"fee"`
}

type TssReq struct {
	Data  []byte
	Index int
}

// Instance hold the oppy_bridge entity
type Instance struct {
	EthClient            *ethclient.Client
	ethClientLocker      *sync.RWMutex
	configAddr           string
	chainID              *big.Int
	tokenAbi             *abi.ABI
	logger               zerolog.Logger
	lastTwoPools         []*bcommon.PoolInfo
	poolLocker           *sync.RWMutex
	tssServer            tssclient.TssInstance
	InboundReqChan       chan []*bcommon.InBoundReq
	RetryInboundReq      *sync.Map // if a tx fail to process, we need to put in this channel and wait for retry
	moveFundReq          *sync.Map
	CurrentHeight        int64
	TokenList            tokenlist.BridgeTokenListI
	wg                   *sync.WaitGroup
	SubChannelNow        chan *types.Header
	ChannelQueue         chan *types.Header
	SubHandler           ethereum.Subscription
	onHoldRetryQueue     []*bcommon.InBoundReq
	onHoldRetryQueueLock *sync.Mutex
}

// NewChainInstance initialize the oppy_bridge entity
func NewChainInstance(ws string, tssServer tssclient.TssInstance, tl tokenlist.BridgeTokenListI, wg *sync.WaitGroup) (*Instance, error) {
	logger := log.With().Str("module", "pubchain").Logger()

	ethClient, err := ethclient.Dial(ws)
	if err != nil {
		logger.Error().Err(err).Msg("fail to dial the websocket")
		return nil, errors.New("fail to dial the network")
	}

	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	chainID, err := ethClient.NetworkID(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("fail to get the chain ID")
		return nil, err
	}

	tAbi, err := abi.JSON(strings.NewReader(generated.GeneratedMetaData.ABI))
	if err != nil {
		return nil, fmt.Errorf("fail to get the tokenABI with err %v", err)
	}

	return &Instance{
		logger:               logger,
		EthClient:            ethClient,
		ethClientLocker:      &sync.RWMutex{},
		configAddr:           ws,
		chainID:              chainID,
		tokenAbi:             &tAbi,
		poolLocker:           &sync.RWMutex{},
		tssServer:            tssServer,
		lastTwoPools:         make([]*bcommon.PoolInfo, 2),
		InboundReqChan:       make(chan []*bcommon.InBoundReq, inboundprosSize),
		RetryInboundReq:      &sync.Map{},
		moveFundReq:          &sync.Map{},
		TokenList:            tl,
		wg:                   wg,
		ChannelQueue:         make(chan *types.Header, sbchannelsize),
		onHoldRetryQueue:     []*bcommon.InBoundReq{},
		onHoldRetryQueueLock: &sync.Mutex{},
	}, nil
}
