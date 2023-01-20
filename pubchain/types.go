package pubchain

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	cossubmit "gitlab.com/joltify/joltifychain-bridge/cos_submit"

	"github.com/cosmos/cosmos-sdk/simapp/params"
	grpc1 "github.com/gogo/protobuf/grpc"
	"gitlab.com/joltify/joltifychain-bridge/config"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/joltify/joltifychain-bridge/tokenlist"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/rs/zerolog"
	"gitlab.com/joltify/joltifychain-bridge/generated"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog/log"

	"github.com/ethereum/go-ethereum/core/types"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
)

const (
	inboundprosSize   = 512
	sbchannelsize     = 20000
	chainQueryTimeout = time.Second * 15
	ROUNDBLOCK        = 50
	submitBackoff     = time.Second * 4
	GroupBlockGap     = 8
	GroupSign         = 4
	channelSize       = 2000
	movefundretrygap  = 3
	ETH               = "ETH"
	BSC               = "BSC"
	JOLTIFY           = "JOLTIFY"
)

var (
	OppyContractAddressBSC = "0x94277968dff216265313657425d9d7577ad32dd1"
	OppyContractAddressETH = "0x2BCD5745eBf28f367A0De2cF3C6fEBfE42B21338"
)

type JoltHandler interface {
	queryTokenPrice(grpcClient grpc1.ClientConn, grpcAddr string, denom string) (sdk.Dec, error)
	QueryJoltBlockHeight(grpcAddr string) (int64, error)
}

type InboundTx struct {
	TxID            string         `json:"tx_id"` // this variable is used for locally saving and loading
	ReceiverAddress sdk.AccAddress `json:"receiver_address"`
	PubBlockHeight  uint64         `json:"pub_block_height"` // this variable is used to delete the expired tx
	Token           sdk.Coin       `json:"token"`
}

type Erc20TxInfo struct {
	receiverAddr      sdk.AccAddress // address that we should send token to
	receiverAddrERC20 string         // address that we should send token to
	toAddr            common.Address // address to the pool
	Amount            *big.Int       // the amount of the token that cross the bridge
	tokenAddress      common.Address // the erc20 contract address (we need to ensure the token address is 100% correct)
	dstChainType      string
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

type BlockHead struct {
	Head      *types.Header
	ChainType string
}

type Erc20ChainInfo struct {
	WsAddr          string
	contractAddress string
	ChainType       string
	Client          *ethclient.Client
	ChainLocker     *sync.RWMutex
	ChainID         *big.Int
	logger          zerolog.Logger
	wg              *sync.WaitGroup // this must be the wg of the pubchain instance thus te whole process can be contolled by the main progress
	SubChannelNow   chan *types.Header
	SubHandler      ethereum.Subscription
	ChannelQueue    chan *BlockHead
}

type CosMosChainInfo struct {
	ChainType    string
	encoding     *params.EncodingConfig
	logger       zerolog.Logger
	wg           *sync.WaitGroup // this must be the wg of the pubchain instance thus te whole process can be contolled by the main progress
	ChannelQueue chan *BlockHead
	CosHandler   *cossubmit.CosHandler
}

func NewCosChainInfo(grpcAddr, httpAddr, chainType string, wg *sync.WaitGroup, tssServer tssclient.TssInstance) (*CosMosChainInfo, error) {
	enc := bcommon.MakeEncodingConfig()
	key := keyring.NewInMemory()
	handler, err := cossubmit.NewCosOperations(grpcAddr, httpAddr, nil, key, "joltify", tssServer)
	if err != nil {
		return nil, err
	}

	c := CosMosChainInfo{
		logger:     log.With().Str("module", chainType).Logger(),
		ChainType:  chainType,
		wg:         wg,
		encoding:   &enc,
		CosHandler: handler,
	}
	return &c, nil
}

func NewErc20ChainInfo(wsAddress, chainType string, wg *sync.WaitGroup) (*Erc20ChainInfo, error) {
	client, err := ethclient.Dial(wsAddress)
	if err != nil {
		return nil, errors.New("fail to dial the network")
	}

	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	// fixme should use chainID
	// chainID, err := client.NetworkID(ctx)
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return nil, err
	}

	c := Erc20ChainInfo{
		WsAddr:      wsAddress,
		ChainType:   chainType,
		Client:      client,
		ChainID:     chainID,
		ChainLocker: &sync.RWMutex{},
		logger:      log.With().Str("module", chainType).Logger(),
		wg:          wg,
	}
	return &c, nil
}

func (ercChain *Erc20ChainInfo) Terminate() {
	ercChain.Client.Close()
}

// Instance hold the oppy_bridge entity
type Instance struct {
	BSCChain             *Erc20ChainInfo
	EthChain             *Erc20ChainInfo
	CosChain             *CosMosChainInfo
	tokenAbi             *abi.ABI
	logger               zerolog.Logger
	lastTwoPools         []*bcommon.PoolInfo
	poolLocker           *sync.RWMutex
	tssServer            tssclient.TssInstance
	InboundReqChan       chan []*bcommon.InBoundReq
	RetryInboundReq      *sync.Map // if a tx fail to process, we need to put in this channel and wait for retry
	moveFundReq          *sync.Map
	TokenList            tokenlist.BridgeTokenListI
	wg                   *sync.WaitGroup
	ChannelQueue         chan *BlockHead
	onHoldRetryQueue     []*bcommon.InBoundReq
	onHoldRetryQueueLock *sync.Mutex
	RetryOutboundReq     *sync.Map // if a tx fail to process, we need to put in this channel and wait for retry
	FeeModule            map[string]*bcommon.FeeModule
	joltHandler          JoltHandler
	joltRetryOutBoundReq *sync.Map
}

// NewChainInstance initialize the oppy_bridge entity
func NewChainInstance(cfg config.Config, tssServer tssclient.TssInstance, tl tokenlist.BridgeTokenListI, wg *sync.WaitGroup, joltRetryOutBoundReq *sync.Map) (*Instance, error) {
	logger := log.With().Str("module", "pubchain").Logger()

	retryPools := bcommon.NewRetryPools()

	channelQueue := make(chan *BlockHead, sbchannelsize)

	bscChainClient, err := NewErc20ChainInfo(cfg.PubChainConfig.WsAddressBSC, "BSC", wg)
	if err != nil {
		logger.Error().Err(err).Msg("fail to create the eth chain client")
		return nil, errors.New("invalid eth client")
	}
	bscChainClient.ChannelQueue = channelQueue
	bscChainClient.contractAddress = OppyContractAddressBSC

	ethChainClient, err := NewErc20ChainInfo(cfg.PubChainConfig.WsAddressETH, "ETH", wg)
	if err != nil {
		logger.Error().Err(err).Msg("fail to create the eth chain client")
		return nil, errors.New("invalid eth client")
	}
	ethChainClient.ChannelQueue = channelQueue
	ethChainClient.contractAddress = OppyContractAddressETH

	// add atom chain
	atomClient, err := NewCosChainInfo(cfg.AtomChain.GrpcAddress, cfg.AtomChain.HTTPAddress, "ATOM", wg, tssServer)
	if err != nil {
		logger.Error().Err(err).Msgf("fail to create the atom client")
		return nil, err
	}
	atomClient.ChannelQueue = channelQueue

	tAbi, err := abi.JSON(strings.NewReader(generated.GeneratedMetaData.ABI))
	if err != nil {
		return nil, fmt.Errorf("fail to get the tokenABI with err %v", err)
	}

	return &Instance{
		logger:               logger,
		BSCChain:             bscChainClient,
		EthChain:             ethChainClient,
		CosChain:             atomClient,
		tokenAbi:             &tAbi,
		poolLocker:           &sync.RWMutex{},
		tssServer:            tssServer,
		lastTwoPools:         make([]*bcommon.PoolInfo, 2),
		InboundReqChan:       make(chan []*bcommon.InBoundReq, inboundprosSize),
		RetryInboundReq:      &sync.Map{},
		moveFundReq:          &sync.Map{},
		TokenList:            tl,
		ChannelQueue:         channelQueue,
		wg:                   wg,
		onHoldRetryQueue:     []*bcommon.InBoundReq{},
		onHoldRetryQueueLock: &sync.Mutex{},
		RetryOutboundReq:     retryPools.RetryOutboundReq,
		FeeModule:            bcommon.InitFeeModule(),
		joltHandler:          NewJoltHandler(),
		joltRetryOutBoundReq: joltRetryOutBoundReq,
	}, nil
}

func (pi *Instance) SetKey(uid string, data, pass []byte) error {
	return pi.CosChain.CosHandler.SetKey(uid, data, pass)
}

func (pi *Instance) GetCurrentNewBlockChain() <-chan ctypes.ResultEvent {
	return pi.CosChain.CosHandler.GetCurrentNewBlockChain()
}

func (pi *Instance) GetChannelQueueNewBlockChain() chan ctypes.ResultEvent {
	return pi.CosChain.CosHandler.GetChannelQueueNewBlockChain()
}
