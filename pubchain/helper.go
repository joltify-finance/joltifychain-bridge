package pubchain

import (
	"context"
	"errors"
	"math/big"
	"time"

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

func (pi *Instance) CheckPubChainHealthWithLock() error {
	pi.ethClientLocker.RLock()
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	_, err := pi.EthClient.BlockByNumber(ctx, nil)
	pi.ethClientLocker.RUnlock()
	return err
}

func (pi *Instance) GetBlockByNumberWithLock(number *big.Int) (*types.Block, error) {
	pi.ethClientLocker.RLock()
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	block, err := pi.EthClient.BlockByNumber(ctx, number)
	pi.ethClientLocker.RUnlock()

	if err != nil {
		// we reset the ethcliet
		pi.logger.Error().Err(err).Msgf("error of the ethclient")
		pi.wg.Add(1)
		go func() {
			defer pi.wg.Done()
			errInner := pi.RetryPubChain()
			if errInner != nil {
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

	if err != nil && err.Error() != "not found" {
		// we reset the ethcliet
		pi.logger.Error().Err(err).Msgf("error of the ethclient")
		pi.wg.Add(1)
		go func() {
			defer pi.wg.Done()
			errInner := pi.RetryPubChain()
			if errInner != nil {
				pi.logger.Error().Err(errInner).Msgf("we fail to restart the eth client")
			}
		}()
	}
	return receipt, err
}

func (pi *Instance) GetFeeLimitWithLock() (*big.Int, *big.Int, int64, uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	pi.ethClientLocker.RLock()
	gasPrice, err1 := pi.EthClient.SuggestGasPrice(ctx)
	mockMsg := ethereum.CallMsg{
		From:     common.HexToAddress("0x2cDaa3f15f3db73a6E75c462975EBFca9B5A56ce"),
		To:       nil,
		GasPrice: gasPrice,
		Value:    big.NewInt(2),
		// Data:     []byte("hello"),
	}
	gas, err2 := pi.EthClient.EstimateGas(ctx, mockMsg)
	pi.ethClientLocker.RUnlock()
	if err1 != nil || err2 != nil {
		// we reset the ethcliet
		pi.logger.Error().Err(err1).Msgf("error of the gas price estimate")
		pi.logger.Error().Err(err2).Msgf("error of the gas estimate")
		pi.wg.Add(1)
		go func() {
			defer pi.wg.Done()
			err := pi.RetryPubChain()
			if err != nil {
				pi.logger.Error().Err(err).Msgf("we fail to restart the eth client")
			}
		}()
		return big.NewInt(0), big.NewInt(0), 0, 0, errors.New("fail to get the fee")
	}

	adjGas := int64(float32(gas) * config.PubChainGASFEERATIO)
	totalFee := new(big.Int).Mul(gasPrice, big.NewInt(adjGas))
	return totalFee, gasPrice, adjGas, gas, nil
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
			err := pi.RetryPubChain()
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

func (pi *Instance) renewEthClientWithLock(ethclient *ethclient.Client) error {
	pi.ethClientLocker.Lock()
	// release the old one
	if pi.SubHandler != nil {
		pi.SubHandler.Unsubscribe()
	}
	pi.EthClient.Close()
	pi.EthClient = ethclient

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	err := pi.UpdateSubscription(ctx)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("we fail to update the pubchain subscription")
		pi.ethClientLocker.Unlock()
		return err
	}
	pi.ethClientLocker.Unlock()
	return nil
}

func (pi *Instance) HealthCheckAndReset() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	_, err := pi.EthClient.BlockNumber(ctx)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("public chain connnection seems stopped we reset")
		err2 := pi.RetryPubChain()
		if err2 != nil {
			pi.logger.Error().Err(err).Msgf("pubchain fail to restart")
		}
	}
}

// CheckTxStatus check whether the tx is already in the chain
func (pi *Instance) CheckTxStatus(hashStr string) error {
	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(submitBackoff), 30)

	var status uint64
	op := func() error {
		txHash := common.HexToHash(hashStr)
		ret, err := pi.checkEachTx(txHash)
		if err != nil {
			return err
		}
		status = ret
		return nil
	}

	err := backoff.Retry(op, bf)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to find the tx %v", hashStr)
		return err
	}
	if status != 1 {
		pi.logger.Warn().Msgf("the tx is failed, we need to redo the tx")
		return errors.New("tx failed")
	}
	pi.logger.Info().Msgf("we have successfully check the tx.")
	return nil
}

func (pi *Instance) checkEachTx(h common.Hash) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.QueryTimeOut)
	defer cancel()
	receipt, err := pi.getTransactionReceiptWithLock(ctx, h)
	if err != nil {
		return 0, err
	}
	return receipt.Status, nil
}

func (pi *Instance) getBalance(value *big.Int) (types2.Coin, error) {
	total := types2.NewCoin(config.OutBoundDenomFee, types2.NewIntFromBigInt(value))
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
		pi.logger.Error().Err(err).Msg("fail to recover the oppy Address")
		return types2.AccAddress{}, err
	}
	return transferFrom, nil
}
