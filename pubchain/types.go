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
	inboundprosSize     = 512
	sbchannelsize       = 20000
	chainQueryTimeout   = time.Second * 15
	GasLimit            = 2100000
	GasPrice            = "0.00000001"
	ROUNDBLOCK          = 50
	submitBackoff       = time.Second * 4
	GroupBlockGap       = 8
	GroupSign           = 4
	PRICEUPDATEGAP      = 10
	OppyContractAddress = "0x94277968dff216265313657425d9d7577ad32dd1"
	movefundretrygap    = 3
)

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

// Instance hold the oppy_bridge entity
type Instance struct {
	EthClient       *ethclient.Client
	ethClientLocker *sync.RWMutex
	configAddr      string
	chainID         *big.Int
	tokenAbi        *abi.ABI
	logger          zerolog.Logger
	lastTwoPools    []*bcommon.PoolInfo
	poolLocker      *sync.RWMutex
	tssServer       tssclient.TssInstance
	InboundReqChan  chan *bcommon.InBoundReq
	RetryInboundReq *sync.Map // if a tx fail to process, we need to put in this channel and wait for retry
	moveFundReq     *sync.Map
	CurrentHeight   int64
	TokenList       tokenlist.TokenListI
	wg              *sync.WaitGroup
	SubChannelNow   chan *types.Header
	ChannelQueue    chan *types.Header
	SubHandler      ethereum.Subscription
}

// NewChainInstance initialize the oppy_bridge entity
func NewChainInstance(ws string, tssServer tssclient.TssInstance, tl tokenlist.TokenListI, wg *sync.WaitGroup) (*Instance, error) {
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
		logger:          logger,
		EthClient:       ethClient,
		ethClientLocker: &sync.RWMutex{},
		configAddr:      ws,
		chainID:         chainID,
		tokenAbi:        &tAbi,
		poolLocker:      &sync.RWMutex{},
		tssServer:       tssServer,
		lastTwoPools:    make([]*bcommon.PoolInfo, 2),
		InboundReqChan:  make(chan *bcommon.InBoundReq, inboundprosSize),
		RetryInboundReq: &sync.Map{},
		moveFundReq:     &sync.Map{},
		TokenList:       tl,
		wg:              wg,
		ChannelQueue:    make(chan *types.Header, sbchannelsize),
	}, nil
}

func (pi *Instance) getBlockByNumberWithLock(ctx context.Context, number *big.Int) (*types.Block, error) {
	pi.ethClientLocker.RLock()
	block, err := pi.EthClient.BlockByNumber(ctx, number)
	pi.ethClientLocker.RUnlock()

	if err != nil {
		// we reset the ethcliet
		pi.logger.Error().Err(err).Msgf("error of the ethclient")
		pi.wg.Add(1)
		go func() {
			defer pi.wg.Done()
			err = pi.RetryPubChain()
			if err != nil {
				pi.logger.Error().Err(err).Msgf("we fail to restart the eth client")
			}
		}()
	}
	return block, err
}

func (pi *Instance) getTransactionReceiptWithLock(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	pi.ethClientLocker.RLock()
	receipt, err := pi.EthClient.TransactionReceipt(ctx, txHash)
	pi.ethClientLocker.RUnlock()

	if err != nil {
		// we reset the ethcliet
		pi.logger.Error().Err(err).Msgf("error of the ethclient")
		pi.wg.Add(1)
		go func() {
			defer pi.wg.Done()
			err = pi.RetryPubChain()
			if err != nil {
				pi.logger.Error().Err(err).Msgf("we fail to restart the eth client")
			}
		}()
	}
	return receipt, err
}

func (pi *Instance) GetGasPriceWithLock() (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	pi.ethClientLocker.RLock()
	gasPrice, err := pi.EthClient.SuggestGasPrice(ctx)
	pi.ethClientLocker.RUnlock()

	if err != nil {
		// we reset the ethcliet
		pi.logger.Error().Err(err).Msgf("error of the ethclient")
		pi.wg.Add(1)
		go func() {
			defer pi.wg.Done()
			err = pi.RetryPubChain()
			if err != nil {
				pi.logger.Error().Err(err).Msgf("we fail to restart the eth client")
			}
		}()
	}
	return gasPrice, err
}

func (pi *Instance) getBalanceWithLock(ctx context.Context, ethAddr common.Address) (*big.Int, error) {
	pi.ethClientLocker.RLock()
	balanceBnB, err := pi.EthClient.BalanceAt(ctx, ethAddr, nil)
	pi.ethClientLocker.RUnlock()

	if err != nil {
		// we reset the ethcliet
		pi.logger.Error().Err(err).Msgf("error of the ethclient")
		pi.wg.Add(1)
		go func() {
			defer pi.wg.Done()
			err = pi.RetryPubChain()
			if err != nil {
				pi.logger.Error().Err(err).Msgf("we fail to restart the eth client")
			}
		}()
	}
	return balanceBnB, err
}

func (pi *Instance) getPendingNonceWithLock(ctx context.Context, poolAddress common.Address) (uint64, error) {
	pi.ethClientLocker.RLock()
	nonce, err := pi.EthClient.PendingNonceAt(ctx, poolAddress)
	pi.ethClientLocker.RUnlock()

	if err != nil {
		// we reset the ethcliet
		pi.logger.Error().Err(err).Msgf("fail to get the pending nonce ")
		pi.wg.Add(1)
		go func() {
			defer pi.wg.Done()
			err := pi.RetryPubChain()
			if err != nil {
				pi.logger.Error().Err(err).Msgf("fail to reset the public chain")

			}
		}()
	}

	return nonce, err
}

func (pi *Instance) sendTransactionWithLock(ctx context.Context, tx *types.Transaction) error {
	pi.ethClientLocker.RLock()
	err := pi.EthClient.SendTransaction(ctx, tx)
	pi.ethClientLocker.RUnlock()

	if err != nil {
		if err.Error() == "already known" {
			pi.logger.Warn().Err(err).Msgf("the tx has already known online")
			return nil
		}
		// we reset the ethcliet
		pi.logger.Error().Err(err).Msgf("fail to send transactions")
		pi.wg.Add(1)
		go func() {
			defer pi.wg.Done()
			err := pi.RetryPubChain()
			if err != nil {
				pi.logger.Error().Err(err).Msgf("fail to reset the public chain")

			}
		}()
	}
	return err
}

func (pi *Instance) renewEthClientWithLock(ethclient *ethclient.Client) {
	pi.ethClientLocker.Lock()
	pi.EthClient.Close()
	pi.EthClient = ethclient
	pi.ethClientLocker.Unlock()
}
