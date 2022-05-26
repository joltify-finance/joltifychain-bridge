package pubchain

import (
	"context"
	"errors"
	"fmt"
	"gitlab.com/joltify/joltifychain-bridge/tokenlist"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/rs/zerolog"
	"gitlab.com/joltify/joltifychain-bridge/generated"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog/log"

	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
)

const (
	inboundprosSize   = 512
	sbchannelsize     = 20000
	chainQueryTimeout = time.Second * 15
	GasLimit          = 2100000
	GasPrice          = "0.00000001"
	ROUNDBLOCK        = 50
	submitBackoff     = time.Second * 4
	GroupBlockGap     = 8
	GroupSign         = 4
)

type InboundTx struct {
	TxID           string         `json:"tx_id"` // this variable is used for locally saving and loading
	Address        sdk.AccAddress `json:"address"`
	PubBlockHeight uint64         `json:"pub_block_height"` // this variable is used to delete the expired tx
	Token          sdk.Coin       `json:"token"`
	Fee            sdk.Coin       `json:"fee"`
}

type InboundTxBnb struct {
	BlockHeight uint64   `json:"block_height"`
	TxID        string   `json:"tx_id"`
	Fee         sdk.Coin `json:"fee"`
}

// Instance hold the joltify_bridge entity
type Instance struct {
	EthClient          *ethclient.Client
	configAddr         string
	chainID            *big.Int
	tokenAbi           *abi.ABI
	logger             zerolog.Logger
	pendingInbounds    *sync.Map
	pendingInboundsBnB *sync.Map
	lastTwoPools       []*bcommon.PoolInfo
	poolLocker         *sync.RWMutex
	tssServer          tssclient.TssInstance
	InboundReqChan     chan *bcommon.InBoundReq
	RetryInboundReq    *sync.Map // if a tx fail to process, we need to put in this channel and wait for retry
	moveFundReq        *sync.Map
	CurrentHeight      int64
	TokenList          *tokenlist.TokenList
}

// NewChainInstance initialize the joltify_bridge entity
func NewChainInstance(ws string, tssServer tssclient.TssInstance, tl *tokenlist.TokenList) (*Instance, error) {
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

	tAbi, err := abi.JSON(strings.NewReader(generated.TokenMetaData.ABI))
	if err != nil {
		return nil, fmt.Errorf("fail to get the tokenABI with err %v", err)
	}

	return &Instance{
		logger:             logger,
		EthClient:          ethClient,
		configAddr:         ws,
		chainID:            chainID,
		tokenAbi:           &tAbi,
		pendingInbounds:    new(sync.Map),
		pendingInboundsBnB: new(sync.Map),
		poolLocker:         &sync.RWMutex{},
		tssServer:          tssServer,
		lastTwoPools:       make([]*bcommon.PoolInfo, 2),
		InboundReqChan:     make(chan *bcommon.InBoundReq, inboundprosSize),
		RetryInboundReq:    &sync.Map{},
		moveFundReq:        &sync.Map{},
		TokenList:          tl,
	}, nil
}
