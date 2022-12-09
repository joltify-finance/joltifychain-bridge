package pubchain

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"time"

	zlog "github.com/rs/zerolog/log"

	"github.com/cenkalti/backoff"
	types2 "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
)

func (c *ChainInfo) CheckChainHealthWithLock() error {
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

func (c *ChainInfo) GetBlockByNumberWithLock(number *big.Int) (*types.Block, error) {
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

func (c *ChainInfo) getTransactionReceiptWithLock(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
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

func (c *ChainInfo) GetFeeLimitWithLock() (*big.Int, *big.Int, int64, uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	c.ChainLocker.RLock()
	gasPrice, err1 := c.Client.SuggestGasPrice(ctx)
	mockMsg := ethereum.CallMsg{
		From:     common.HexToAddress("0x2cDaa3f15f3db73a6E75c462975EBFca9B5A56ce"),
		To:       nil,
		GasPrice: gasPrice,
		Value:    big.NewInt(2),
		// Data:     []byte("hello"),
	}
	gas, err2 := c.Client.EstimateGas(ctx, mockMsg)
	c.ChainLocker.RUnlock()
	if err1 != nil || err2 != nil {
		// we reset the ethcliet
		c.logger.Error().Err(err1).Msgf("error of the gas price estimate")
		c.logger.Error().Err(err2).Msgf("error of the gas estimate")
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			err := c.RetryPubChain()
			if err != nil {
				c.logger.Error().Err(err).Msgf("we fail to restart the eth client")
			}
		}()
		return big.NewInt(0), big.NewInt(0), 0, 0, errors.New("fail to get the fee")
	}

	adjGas := int64(float32(gas) * config.PubChainGASFEERATIO)
	totalFee := new(big.Int).Mul(gasPrice, big.NewInt(adjGas))
	return totalFee, gasPrice, adjGas, gas, nil
}

func (c *ChainInfo) GetGasPriceWithLock() (*big.Int, error) {
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

func (c *ChainInfo) getBalanceWithLock(ctx context.Context, ethAddr common.Address) (*big.Int, error) {
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

func (c *ChainInfo) getPendingNonceWithLock(ctx context.Context, poolAddress common.Address) (uint64, error) {
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

func (c *ChainInfo) sendTransactionWithLock(ctx context.Context, tx *types.Transaction) error {
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

func (c *ChainInfo) renewEthClientWithLock(ethclient *ethclient.Client) error {
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

func (c *ChainInfo) HealthCheckAndReset() {
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
func (c *ChainInfo) CheckTxStatus(hashStr string) error {
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
		c.logger.Warn().Msgf("the tx is failed, we need to redo the tx")
		return errors.New("tx failed")
	}
	c.logger.Info().Msgf("we have successfully check the tx.")
	return nil
}

func (c *ChainInfo) checkEachTx(h common.Hash) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.QueryTimeOut)
	defer cancel()
	receipt, err := c.getTransactionReceiptWithLock(ctx, h)
	if err != nil {
		return 0, err
	}
	return receipt.Status, nil
}

func (pi *Instance) getBalance(value *big.Int, denom string) (types2.Coin, error) {
	total := types2.NewCoin(denom, types2.NewIntFromBigInt(value))
	if total.IsNegative() {
		pi.logger.Error().Msg("incorrect amount")
		return types2.Coin{}, errors.New("insufficient fund")
	}
	return total, nil
}

func (pi *Instance) retrieveAddrfromRawTx(tx *types.Transaction) (types2.AccAddress, error) { //nolint
	v, r, s := tx.RawSignatureValues()
	signer := types.LatestSignerForChainID(tx.ChainId())
	plainV := misc.RecoverRecID(tx.ChainId().Uint64(), v)
	sigBytes := misc.MakeSignature(r, s, plainV)

	sigPublicKey, err := crypto.Ecrecover(signer.Hash(tx).Bytes(), sigBytes)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to recover the public key")
		return types2.AccAddress{}, err
	}

	transferFrom, err := misc.EthSignPubKeyToOppyAddr(sigPublicKey)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to recover the oppy ReceiverAddress")
		return types2.AccAddress{}, err
	}
	return transferFrom, nil
}

func (pi *Instance) GetChainClient(chainType string) *ChainInfo {
	var chainInfo *ChainInfo
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
