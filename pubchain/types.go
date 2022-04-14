package pubchain

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
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

type inboundTx struct {
	address        sdk.AccAddress
	pubBlockHeight uint64 // this variable is used to delete the expired tx
	token          sdk.Coin
	fee            sdk.Coin
}

type inboundTxBnb struct {
	blockHeight uint64
	txID        string
	fee         sdk.Coin
}

// Instance hold the joltify_bridge entity
type Instance struct {
	EthClient          *ethclient.Client
	configAddr         string
	chainID            *big.Int
	tokenAddr          string
	tokenInstance      *generated.Token
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
}

// NewChainInstance initialize the joltify_bridge entity
func NewChainInstance(ws, tokenAddr string, tssServer tssclient.TssInstance) (*Instance, error) {
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

	tokenIns, err := generated.NewToken(common.HexToAddress(tokenAddr), ethClient)
	if err != nil {
		return nil, errors.New("fail to get the new token")
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
		tokenAddr:          tokenAddr,
		tokenInstance:      tokenIns,
		tokenAbi:           &tAbi,
		pendingInbounds:    new(sync.Map),
		pendingInboundsBnB: new(sync.Map),
		poolLocker:         &sync.RWMutex{},
		tssServer:          tssServer,
		lastTwoPools:       make([]*bcommon.PoolInfo, 2),
		InboundReqChan:     make(chan *bcommon.InBoundReq, inboundprosSize),
		RetryInboundReq:    &sync.Map{},
		moveFundReq:        &sync.Map{},
	}, nil
}
