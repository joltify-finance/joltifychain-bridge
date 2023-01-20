package pubchain

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"

	grpc1 "github.com/gogo/protobuf/grpc"
	pricefeedtypes "github.com/joltify-finance/joltify_lending/x/third_party/pricefeed/types"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	"google.golang.org/grpc"

	zlog "github.com/rs/zerolog/log"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cenkalti/backoff"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

func (c *Erc20ChainInfo) CheckChainHealthWithLock() error {
	c.ChainLocker.RLock()
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	_, err := c.Client.BlockByNumber(ctx, nil)
	c.ChainLocker.RUnlock()
	return err
}

func (pi *Instance) CheckPubChainHealthWithLock() error {
	wg := sync.WaitGroup{}
	wg.Add(2)
	var err error
	go func() {
		defer wg.Done()
		err1 := pi.BSCChain.CheckChainHealthWithLock()
		if err1 != nil {
			err = err1
		}
	}()

	go func() {
		defer wg.Done()
		err2 := pi.EthChain.CheckChainHealthWithLock()
		if err2 != nil {
			err = err2
		}
	}()
	wg.Wait()
	return err
}

func (c *Erc20ChainInfo) GetBlockByNumberWithLock(number *big.Int) (*types.Block, error) {
	c.ChainLocker.RLock()
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	block, err := c.Client.BlockByNumber(ctx, number)
	c.ChainLocker.RUnlock()

	if err != nil {
		// we reset the ethcliet
		c.logger.Error().Err(err).Msgf("error of the ethclient")
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			errInner := c.RetryPubChain()
			if errInner != nil {
				c.logger.Error().Err(err).Msgf("we fail to restart the eth client")
			}
		}()
	}
	return block, err
}

func (c *Erc20ChainInfo) getTransactionReceiptWithLock(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	c.ChainLocker.RLock()
	receipt, err := c.Client.TransactionReceipt(ctx, txHash)
	c.ChainLocker.RUnlock()

	if err != nil && err.Error() != "not found" {
		// we reset the ethcliet
		c.logger.Error().Err(err).Msgf("error of the ethclient")
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			errInner := c.RetryPubChain()
			if errInner != nil {
				c.logger.Error().Err(errInner).Msgf("we fail to restart the eth client")
			}
		}()
	}
	return receipt, err
}

// GetFeeLimitWithLock returns fee, gasprice, gaslimit and error code
func (c *Erc20ChainInfo) GetFeeLimitWithLock() (*big.Int, *big.Int, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	c.ChainLocker.RLock()
	gasPrice, err1 := c.Client.SuggestGasPrice(ctx)
	c.ChainLocker.RUnlock()
	if err1 != nil {
		// we reset the ethcliet
		c.logger.Error().Err(err1).Msgf("error of the gas price estimate")
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			err := c.RetryPubChain()
			if err != nil {
				c.logger.Error().Err(err).Msgf("we fail to restart the eth client")
			}
		}()
		return big.NewInt(0), big.NewInt(0), 0, errors.New("fail to get the fee")
	}
	totalFee := new(big.Int).Mul(gasPrice, big.NewInt(config.DEFAULTNATIVEGAS))
	return totalFee, gasPrice, config.DEFAULTNATIVEGAS, nil
}

func (c *Erc20ChainInfo) GetGasPriceWithLock() (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	c.ChainLocker.RLock()
	gasPrice, err := c.Client.SuggestGasPrice(ctx)
	c.ChainLocker.RUnlock()

	if err != nil {
		// we reset the ethcliet
		c.logger.Error().Err(err).Msgf("error of the ethclient")
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			err = c.RetryPubChain()
			if err != nil {
				c.logger.Error().Err(err).Msgf("we fail to restart the eth client")
			}
		}()
	}
	return gasPrice, err
}

func (c *Erc20ChainInfo) getBalanceWithLock(ctx context.Context, ethAddr common.Address) (*big.Int, error) {
	c.ChainLocker.RLock()
	balanceBnB, err := c.Client.BalanceAt(ctx, ethAddr, nil)
	c.ChainLocker.RUnlock()

	if err != nil {
		// we reset the ethcliet
		c.logger.Error().Err(err).Msgf("error of the ethclient")
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			err := c.RetryPubChain()
			if err != nil {
				c.logger.Error().Err(err).Msgf("we fail to restart the eth client")
			}
		}()
	}
	return balanceBnB, err
}

func (c *Erc20ChainInfo) getPendingNonceWithLock(ctx context.Context, poolAddress common.Address) (uint64, error) {
	c.ChainLocker.RLock()
	nonce, err := c.Client.PendingNonceAt(ctx, poolAddress)
	c.ChainLocker.RUnlock()

	if err != nil {
		// we reset the ethcliet
		c.logger.Error().Err(err).Msgf("fail to get the pending nonce ")
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			err := c.RetryPubChain()
			if err != nil {
				c.logger.Error().Err(err).Msgf("fail to reset the public chain")
			}
		}()
	}
	return nonce, err
}

func (c *Erc20ChainInfo) sendTransactionWithLock(ctx context.Context, tx *types.Transaction) error {
	c.ChainLocker.RLock()
	err := c.Client.SendTransaction(ctx, tx)
	c.ChainLocker.RUnlock()

	if err != nil {
		if err.Error() == "already known" {
			c.logger.Warn().Err(err).Msgf("the tx has already known online")
			return nil
		}
		// we reset the ethcliet
		c.logger.Error().Err(err).Msgf("fail to send transactions")
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			err := c.RetryPubChain()
			if err != nil {
				c.logger.Error().Err(err).Msgf("fail to reset the public chain")
			}
		}()
	}
	return err
}

func (c *Erc20ChainInfo) renewEthClientWithLock(ethclient *ethclient.Client) error {
	c.ChainLocker.Lock()
	// release the old one
	if c.SubHandler != nil {
		c.SubHandler.Unsubscribe()
	}
	c.Client.Close()
	c.Client = ethclient

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	err := c.UpdateSubscription(ctx)
	if err != nil {
		c.logger.Error().Err(err).Msgf("we fail to update the pubchain subscription")
		c.ChainLocker.Unlock()
		return err
	}
	c.ChainLocker.Unlock()
	return nil
}

func (pi *Instance) HealthCheckAndReset() {
	pi.BSCChain.HealthCheckAndReset()
	pi.EthChain.HealthCheckAndReset()
}

func (c *Erc20ChainInfo) HealthCheckAndReset() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	_, err := c.Client.BlockNumber(ctx)
	if err != nil {
		c.logger.Error().Err(err).Msgf("public chain connnection seems stopped we reset")
		err2 := c.RetryPubChain()
		if err2 != nil {
			c.logger.Error().Err(err).Msgf("pubchain fail to restart")
		}
	}
}

// CheckTxStatus check whether the tx is already in the chain
func (c *Erc20ChainInfo) CheckTxStatus(hashStr string) error {
	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(submitBackoff), 30)

	var status uint64
	op := func() error {
		txHash := common.HexToHash(hashStr)
		ret, err := c.checkEachTx(txHash)
		if err != nil {
			return err
		}
		status = ret
		return nil
	}

	err := backoff.Retry(op, bf)
	if err != nil {
		c.logger.Error().Err(err).Msgf("fail to find the tx %v", hashStr)
		return err
	}
	if status != 1 {
		c.logger.Warn().Msgf("the tx is failed, we need to redo the tx with status %v", status)
		return errors.New("tx failed")
	}
	c.logger.Info().Msgf("we have successfully check the tx.")
	return nil
}

func (c *Erc20ChainInfo) checkEachTx(h common.Hash) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.QueryTimeOut)
	defer cancel()
	receipt, err := c.getTransactionReceiptWithLock(ctx, h)
	if err != nil {
		return 0, err
	}
	return receipt.Status, nil
}

func (pi *Instance) getBalance(value *big.Int, denom string) (sdk.Coin, error) {
	total := sdk.NewCoin(denom, sdk.NewIntFromBigInt(value))
	if total.IsNegative() {
		pi.logger.Error().Msg("incorrect amount")
		return sdk.Coin{}, errors.New("insufficient fund")
	}
	return total, nil
}

func (pi *Instance) retrieveAddrfromRawTx(tx *types.Transaction) (sdk.AccAddress, error) { //nolint
	v, r, s := tx.RawSignatureValues()
	signer := types.LatestSignerForChainID(tx.ChainId())
	plainV := misc.RecoverRecID(tx.ChainId().Uint64(), v)
	sigBytes := misc.MakeSignature(r, s, plainV)

	sigPublicKey, err := crypto.Ecrecover(signer.Hash(tx).Bytes(), sigBytes)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to recover the public key")
		return sdk.AccAddress{}, err
	}

	transferFrom, err := misc.EthSignPubKeyToOppyAddr(sigPublicKey)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to recover the joltify ReceiverAddress")
		return sdk.AccAddress{}, err
	}
	return transferFrom, nil
}

func (pi *Instance) GetChainClientERC20(chainType string) *Erc20ChainInfo {
	var chainInfo *Erc20ChainInfo
	switch chainType {
	case ETH:
		chainInfo = pi.EthChain
	case BSC:
		chainInfo = pi.BSCChain
	default:
		zlog.Error().Msgf("invalid chain type")
		return nil
	}
	return chainInfo
}

func inboundAdjust(amount sdk.Int, decimals int, precision int) sdk.Int {
	delta := precision - decimals
	if delta != 0 {
		adjustedTokenAmount := bcommon.AdjustInt(amount, int64(delta))
		return adjustedTokenAmount
	}
	return amount
}

func outboundAdjust(amount sdk.Int, decimals int, precision int) sdk.Int {
	delta := decimals - precision
	if delta != 0 {
		adjustedTokenAmount := bcommon.AdjustInt(amount, int64(delta))
		return adjustedTokenAmount
	}
	return amount
}

type Handler struct{}

func NewJoltHandler() Handler {
	return Handler{}
}

func (Handler) queryTokenPrice(grpcClient grpc1.ClientConn, grpcAddr string, denom string) (sdk.Dec, error) {
	if grpcClient == nil {
		grpcClient2, err := grpc.Dial(grpcAddr, grpc.WithInsecure())
		if err != nil {
			zlog.Logger.Error().Err(err).Msgf("fail to dial the grpc end-point")
			return sdk.Dec{}, err
		}
		defer grpcClient2.Close()
		grpcClient = grpcClient2
	}

	qs := pricefeedtypes.NewQueryClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	var marketID string
	if denom == "ujolt" {
		marketID = "jolt:usd"
	} else {
		marketID = denom[1:] + ":usd"
	}
	req := pricefeedtypes.QueryPriceRequest{MarketId: marketID}

	result, err := qs.Price(ctx, &req)
	if err != nil {
		return sdk.Dec{}, err
	}
	return result.Price.Price, nil
}

func (Handler) QueryJoltBlockHeight(grpcAddr string) (int64, error) {
	grpcClient, err := grpc.Dial(grpcAddr, grpc.WithInsecure())
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to dial the grpc end-point")
		return 0, err
	}
	defer grpcClient.Close()

	ts := tmservice.NewServiceClient(grpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	resp, err := ts.GetLatestBlock(ctx, &tmservice.GetLatestBlockRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Block.Header.Height, nil
}
