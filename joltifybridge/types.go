package joltifybridge

import (
	"go.uber.org/atomic"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/math"
	grpc1 "github.com/gogo/protobuf/grpc"
	"gitlab.com/joltify/joltifychain-bridge/config"

	"github.com/cosmos/cosmos-sdk/simapp/params"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ethereum/go-ethereum/common"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
	"gitlab.com/joltify/joltifychain-bridge/validators"
	"gitlab.com/joltify/joltifychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/rs/zerolog"
	tmclienthttp "github.com/tendermint/tendermint/rpc/client/http"
)

const (
	grpcTimeout   = time.Second * 30
	chainID       = "joltifyChain"
	reqCacheSize  = 1024
	ROUNDBLOCK    = 50
	submitBackoff = time.Millisecond * 500
	GroupBlockGap = 6
	GroupSign     = 8
)

// tssPoolMsg this is the pool pre-submit message for the given height
type tssPoolMsg struct {
	msg         *types.MsgCreateCreatePool
	acc         authtypes.AccountI
	poolPubKey  string
	blockHeight int64
}

func (jc *JoltifyChainInstance) AddMoveFundItem(pool *bcommon.PoolInfo, height int64) {
	jc.moveFundReq.Store(height, pool)
}

// popMoveFundItemAfterBlock pop a move fund item after give block duration
func (jc *JoltifyChainInstance) popMoveFundItemAfterBlock(currentBlockHeight int64) (*bcommon.PoolInfo, int64) {
	min := int64(math.MaxInt64)
	jc.moveFundReq.Range(func(key, value interface{}) bool {
		h := key.(int64)
		if h <= min {
			min = h
		}
		return true
	})

	if min < math.MaxInt64 && (currentBlockHeight-min > config.MINCHECKBLOCKGAP) {
		item, _ := jc.moveFundReq.LoadAndDelete(min)
		return item.(*bcommon.PoolInfo), min
	}
	return nil, 0
}

func (jc *JoltifyChainInstance) PopMoveFundItem() (*bcommon.PoolInfo, int64) {
	min := int64(math.MaxInt64)
	jc.moveFundReq.Range(func(key, value interface{}) bool {
		h := key.(int64)
		if h <= min {
			min = h
		}
		return true
	})
	if min < math.MaxInt64 {
		item, _ := jc.moveFundReq.LoadAndDelete(min)
		return item.(*bcommon.PoolInfo), min
	}
	return nil, 0
}

func (jc *JoltifyChainInstance) ExportItems() []*bcommon.OutBoundReq {
	var items []*bcommon.OutBoundReq
	jc.RetryOutboundReq.Range(func(_, value interface{}) bool {
		items = append(items, value.(*bcommon.OutBoundReq))
		return true
	})
	return items
}

func (jc *JoltifyChainInstance) AddItem(req *bcommon.OutBoundReq) {
	jc.RetryOutboundReq.Store(req.Index(), req)
}

func (jc *JoltifyChainInstance) PopItem(n int) []*bcommon.OutBoundReq {
	var allkeys []*big.Int
	jc.RetryOutboundReq.Range(func(key, value interface{}) bool {
		allkeys = append(allkeys, key.(*big.Int))
		return true
	})

	sort.Slice(allkeys, func(i, j int) bool {
		if allkeys[i].Cmp(allkeys[j]) == -1 {
			return true
		}
		return false
	})
	indexNum := len(allkeys)
	if indexNum == 0 {
		return nil
	}

	returnNum := n
	if indexNum < n {
		returnNum = indexNum
	}

	inboundReqs := make([]*bcommon.OutBoundReq, returnNum)

	for i := 0; i < returnNum; i++ {
		el, loaded := jc.RetryOutboundReq.LoadAndDelete(allkeys[i])
		if !loaded {
			panic("should never fail")
		}
		inboundReqs[i] = el.(*bcommon.OutBoundReq)
	}

	return inboundReqs
}

func (jc *JoltifyChainInstance) Size() int {
	i := 0
	jc.RetryOutboundReq.Range(func(key, value interface{}) bool {
		i += 1
		return true
	})
	return i
}

func (jc *JoltifyChainInstance) ShowItems() {
	jc.RetryOutboundReq.Range(func(key, value interface{}) bool {
		el := value.(*bcommon.OutBoundReq)
		jc.logger.Warn().Msgf("tx in the retry pool %v:%v\n", key, el.TxID)
		return true
	})
}

// JoltifyChainInstance defines the types for joltify pub_chain side
type JoltifyChainInstance struct {
	grpcClient       grpc1.ClientConn
	wsClient         *tmclienthttp.HTTP
	encoding         *params.EncodingConfig
	Keyring          keyring.Keyring
	logger           zerolog.Logger
	validatorSet     *validators.ValidatorSet
	myValidatorInfo  info
	tssServer        tssclient.TssSign
	poolUpdateLocker *sync.RWMutex
	msgSendCache     []tssPoolMsg
	lastTwoPools     []*bcommon.PoolInfo
	OutboundReqChan  chan *bcommon.OutBoundReq
	RetryOutboundReq *sync.Map // if a tx fail to process, we need to put in this channel and wait for retry
	moveFundReq      *sync.Map
	CurrentHeight    int64
	inboundGas       *atomic.Int64
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

type outboundTx struct {
	outReceiverAddress common.Address
	blockHeight        uint64
	token              sdk.Coin
	fee                sdk.Coin
}
