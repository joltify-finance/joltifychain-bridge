package joltifybridge

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain/x/vault/types"
	"go.uber.org/atomic"

	"github.com/cosmos/cosmos-sdk/simapp/params"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
	"gitlab.com/joltify/joltifychain-bridge/validators"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/rs/zerolog"
	tmclienthttp "github.com/tendermint/tendermint/rpc/client/http"
	"google.golang.org/grpc"
)

const (
	grpcTimeout    = time.Second * 10
	chainID        = "joltifyChain"
	reqCacheSize   = 512
	retryCacheSize = 128
)

// tssPoolMsg this is the pool pre-submit message for the given height
type tssPoolMsg struct {
	msg         *types.MsgCreateCreatePool
	acc         authtypes.AccountI
	poolPubKey  string
	blockHeight int64
}

// JoltifyChainBridge defines the types for joltify pub_chain side
type JoltifyChainBridge struct {
	grpcClient       *grpc.ClientConn
	wsClient         *tmclienthttp.HTTP
	encoding         *params.EncodingConfig
	keyring          keyring.Keyring
	logger           zerolog.Logger
	validatorSet     *validators.ValidatorSet
	myValidatorInfo  info
	tssServer        *tssclient.BridgeTssServer
	poolUpdateLocker *sync.RWMutex
	msgSendCache     []tssPoolMsg
	lastTwoPools     []*bcommon.PoolInfo
	pendingOutbounds *sync.Map
	OutboundReqChan  chan *OutBoundReq
	TransferChan     []*<-chan ctypes.ResultEvent
	RetryOutboundReq chan *OutBoundReq // if a tx fail to process, we need to put in this channel and wait for retry
	poolAccInfo      *poolAccInfo
	poolAccLocker    *sync.Mutex
}

type poolAccInfo struct {
	accountNum uint64
	accSeq     *atomic.Uint64
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

// Verify checks whether the outbound tx has paid enough fee
func (a *outboundTx) Verify() error {
	if a.fee.Denom != config.OutBoundDenomFee {
		return errors.New("invalid outbound fee denom")
	}
	amount, err := sdk.NewDecFromStr(config.OUTBoundFeeOut)
	if err != nil {
		return errors.New("invalid minimal inbound fee")
	}
	if a.fee.Amount.LT(sdk.NewIntFromBigInt(amount.BigInt())) {
		return fmt.Errorf("the fee is not enough with %s<%s", a.fee.Amount, amount.String())
	}
	return nil
}

// OutBoundReq is the entity for the outbound tx
type OutBoundReq struct {
	outReceiverAddress common.Address
	fromPoolAddr       common.Address
	coin               sdk.Coin
	blockHeight        int64
}

// GetOutBoundInfo return the outbound tx info
func (o *OutBoundReq) GetOutBoundInfo() (common.Address, common.Address, *big.Int, int64) {
	return o.outReceiverAddress, o.fromPoolAddr, o.coin.Amount.BigInt(), o.blockHeight
}

// SetItemHeight sets the block height of the tx
func (o *OutBoundReq) SetItemHeight(blockHeight int64) {
	o.blockHeight = blockHeight
}

func newAccountOutboundReq(address, fromPoolAddr common.Address, coin sdk.Coin, blockHeight int64) OutBoundReq {
	return OutBoundReq{
		address,
		fromPoolAddr,
		coin,
		blockHeight,
	}
}
