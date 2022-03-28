package pubchain

import (
	"errors"
	"fmt"
	"math/big"
	"sort"
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
	retryCacheSize    = 128
	inboundprosSize   = 512
	sbchannelsize     = 20000
	chainQueryTimeout = time.Second * 5
	GasLimit          = 2100000
	GasPrice          = "0.00000001"
	ROUNDBLOCK        = 50
	submitBackoff     = time.Millisecond * 500
	GroupBlockGap     = 8
	GroupSign         = 4
)

func (pi *Instance) AddItem(req *bcommon.InboundReq) {
	pi.RetryInboundReq.Store(req.Index(), req)
}

func (pi *Instance) PopItem(n int) []*bcommon.InboundReq {
	var allkeys []*big.Int
	pi.RetryInboundReq.Range(func(key, value interface{}) bool {
		allkeys = append(allkeys, key.(*big.Int))
		return true
	})

	sort.Slice(allkeys, func(i, j int) bool {
		return allkeys[i].Cmp(allkeys[j]) == -1
	})
	indexNum := len(allkeys)
	if indexNum == 0 {
		return nil
	}

	returnNum := n
	if indexNum < n {
		returnNum = indexNum
	}

	inboundReqs := make([]*bcommon.InboundReq, returnNum)

	for i := 0; i < returnNum; i++ {
		el, loaded := pi.RetryInboundReq.LoadAndDelete(allkeys[i])
		if !loaded {
			panic("should never fail")
		}
		inboundReqs[i] = el.(*bcommon.InboundReq)
	}

	return inboundReqs
}

func (pi *Instance) Size() int {
	i := 0
	pi.RetryInboundReq.Range(func(key, value interface{}) bool {
		i += 1
		return true
	})
	return i
}

func (pi *Instance) ShowItems() {
	pi.RetryInboundReq.Range(func(key, value interface{}) bool {
		el := value.(*bcommon.InboundReq)
		pi.logger.Warn().Msgf("tx in the prepare pool %v:%v\n", key, el.TxID)
		return true
	})
	return
}

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
	tokenAddr          string
	tokenInstance      *generated.Token
	tokenAbi           *abi.ABI
	logger             zerolog.Logger
	pendingInbounds    *sync.Map
	pendingInboundsBnB *sync.Map
	lastTwoPools       []*bcommon.PoolInfo
	poolLocker         *sync.RWMutex
	tssServer          tssclient.TssSign
	InboundReqChan     chan *bcommon.InboundReq
	RetryInboundReq    *sync.Map // if a tx fail to process, we need to put in this channel and wait for retry
	moveFundReq        *sync.Map
	CurrentHeight      int64
}

// NewChainInstance initialize the joltify_bridge entity
func NewChainInstance(ws, tokenAddr string, tssServer tssclient.TssSign) (*Instance, error) {
	logger := log.With().Str("module", "pubchain").Logger()

	wsClient, err := ethclient.Dial(ws)
	if err != nil {
		logger.Error().Err(err).Msg("fail to dial the websocket")
		return nil, errors.New("fail to dial the network")
	}

	tokenIns, err := generated.NewToken(common.HexToAddress(tokenAddr), wsClient)
	if err != nil {
		return nil, errors.New("fail to get the new token")
	}

	tAbi, err := abi.JSON(strings.NewReader(generated.TokenMetaData.ABI))
	if err != nil {
		return nil, fmt.Errorf("fail to get the tokenABI with err %v", err)
	}

	return &Instance{
		logger:             logger,
		EthClient:          wsClient,
		tokenAddr:          tokenAddr,
		tokenInstance:      tokenIns,
		tokenAbi:           &tAbi,
		pendingInbounds:    new(sync.Map),
		pendingInboundsBnB: new(sync.Map),
		poolLocker:         &sync.RWMutex{},
		tssServer:          tssServer,
		lastTwoPools:       make([]*bcommon.PoolInfo, 2),
		InboundReqChan:     make(chan *bcommon.InboundReq, inboundprosSize),
		RetryInboundReq:    &sync.Map{},
		moveFundReq:        &sync.Map{},
	}, nil
}
