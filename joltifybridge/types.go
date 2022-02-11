package joltifybridge

import (
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	grpc1 "github.com/gogo/protobuf/grpc"

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
	grpcTimeout  = time.Second * 10
	chainID      = "joltifyChain"
	reqCacheSize = 512
)

// tssPoolMsg this is the pool pre-submit message for the given height
type tssPoolMsg struct {
	msg         *types.MsgCreateCreatePool
	acc         authtypes.AccountI
	poolPubKey  string
	blockHeight int64
}

func (pi *JoltifyChainInstance) AddMoveFundItem(pool *bcommon.PoolInfo, height int64) {
	pi.moveFundReq.Store(height, pool)
}

func (pi *JoltifyChainInstance) PopMoveFundItem() (*bcommon.PoolInfo, int64) {
	min := int64(math.MaxInt64)
	pi.moveFundReq.Range(func(key, value interface{}) bool {
		h := key.(int64)
		if h <= min {
			min = h
		}
		return true
	})
	if min < math.MaxInt64 {
		item, _ := pi.moveFundReq.LoadAndDelete(min)
		return item.(*bcommon.PoolInfo), min
	}
	return nil, 0
}

func (pi *JoltifyChainInstance) AddItem(req *OutBoundReq) {
	pi.RetryOutboundReq.Store(req.Hash().Big(), req)
}

func (pi *JoltifyChainInstance) PopItem() *OutBoundReq {
	max := big.NewInt(0)
	pi.RetryOutboundReq.Range(func(key, value interface{}) bool {
		h := key.(*big.Int)
		if max.Cmp(h) == -1 {
			max = h
		}
		return true
	})
	if max.Cmp(big.NewInt(0)) == 1 {
		item, _ := pi.RetryOutboundReq.LoadAndDelete(max)
		return item.(*OutBoundReq)
	}
	return nil
}

func (pi *JoltifyChainInstance) ShowItems() {
	pi.RetryOutboundReq.Range(func(key, value interface{}) bool {
		el := value.(*OutBoundReq)
		pi.logger.Warn().Msgf("tx in the retry pool %v:%v\n", key, el.txID)
		return true
	})
	return
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
	OutboundReqChan  chan *OutBoundReq
	RetryOutboundReq *sync.Map // if a tx fail to process, we need to put in this channel and wait for retry
	poolAccInfo      *poolAccInfo
	poolAccLocker    *sync.Mutex
	moveFundReq      *sync.Map
	broadcastChannel *sync.Map
}

type poolAccInfo struct {
	accountNum uint64
	accSeq     uint64
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

func (i *OutBoundReq) Hash() common.Hash {
	blockHeight := new(big.Int).SetInt64(i.blockHeight)
	hash := crypto.Keccak256Hash(i.outReceiverAddress.Bytes(), i.fromPoolAddr.Bytes(), []byte(i.txID), blockHeight.Bytes())
	return hash
}

// OutBoundReq is the entity for the outbound tx
type OutBoundReq struct {
	txID               string
	outReceiverAddress common.Address
	fromPoolAddr       common.Address
	coin               sdk.Coin
	blockHeight        int64
}

func newOutboundReq(txID string, address, fromPoolAddr common.Address, coin sdk.Coin, blockHeight int64) OutBoundReq {
	return OutBoundReq{
		txID,
		address,
		fromPoolAddr,
		coin,
		blockHeight,
	}
}
