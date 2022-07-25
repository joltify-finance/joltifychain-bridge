package pubchain

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"math/big"
	"strconv"
	"time"

	"gitlab.com/oppy-finance/oppy-bridge/config"

	"github.com/cenkalti/backoff"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	common3 "github.com/oppyfinance/tss/common"
	"gitlab.com/oppy-finance/oppy-bridge/generated"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
)

// CheckTxStatus check whether the tx has been done successfully
func (pi *Instance) waitToSend(poolAddress common.Address, targetNonce uint64) error {
	//bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(submitBackoff), 40)\
	bf := backoff.NewExponentialBackOff()
	bf.MaxElapsedTime = time.Minute * 2
	bf.MaxInterval = time.Second * 20

	alreadyPassed := false
	op := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
		defer cancel()

		nonce, err := pi.getPendingNonceWithLock(ctx, poolAddress)
		if err != nil {
			pi.logger.Error().Err(err).Msgf("fail to get the nonce of the given pool address")
			return err
		}

		if nonce == targetNonce {
			return nil
		}
		if nonce > targetNonce {
			alreadyPassed = true
			return nil
		}
		return fmt.Errorf("not our round, the current nonce is %v and we want %v", nonce, targetNonce)
	}

	err := backoff.Retry(op, bf)
	if alreadyPassed {
		pi.logger.Warn().Msgf("already passed the tx, we quit")
		return errors.New("already passed the seq")
	}
	return err
}

// SendNativeToken sends the native token to the public chain
func (pi *Instance) SendNativeToken(signerPk string, sender, receiver common.Address, amount *big.Int, blockHeight int64, nonce *big.Int) (common.Hash, bool, error) {

	// fixme, we set the fixed gaslimit
	gasLimit, err := strconv.ParseUint(config.DefaultPUBChainGasWanted, 10, 64)
	if err != nil {
		panic("fail to parse the default pubchain gas wanted")
	}
	//fixme need to check what is the gas price here
	gasPrice, err := pi.GetGasPriceWithLock()
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to get the suggested gas price")
		return common.Hash{}, false, err
	}

	totalFee := new(big.Int).Mul(gasPrice, big.NewInt(int64(gasLimit)))
	// this statement is useful in
	if amount.Cmp(totalFee) != 1 {
		return common.Hash{}, true, nil
	}

	if signerPk == "" {
		lastPool := pi.GetPool()[1]
		signerPk = lastPool.Pk
	}
	txo, err := pi.composeTx(signerPk, sender, pi.chainID, blockHeight)
	if err != nil {
		return common.Hash{}, false, err
	}
	if nonce != nil {
		txo.Nonce = nonce
	}

	sendAmount := new(big.Int).Sub(amount, totalFee)
	txo.Value = sendAmount

	var data []byte
	tx := types.NewTx(&types.LegacyTx{Nonce: nonce.Uint64(), GasPrice: gasPrice, Gas: gasLimit, To: &receiver, Value: sendAmount, Data: data})

	signedTx, err := txo.Signer(sender, tx)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to sign the tx")
		return common.Hash{}, false, err
	}

	ctxSend, cancelSend := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancelSend()

	if nonce != nil {
		err = pi.waitToSend(sender, nonce.Uint64())
		if err != nil {
			return signedTx.Hash(), false, err
		}
	}

	err = pi.sendTransactionWithLock(ctxSend, signedTx)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("we fail to send the outbound native tx, have reset the eth client")
	}
	return signedTx.Hash(), false, err
}

// SendToken sends the token to the public chain
func (pi *Instance) SendToken(signerPk string, sender, receiver common.Address, amount *big.Int, blockHeight int64, nonce *big.Int, tokenAddr string) (common.Hash, error) {
	if signerPk == "" {
		lastPool := pi.GetPool()[1]
		signerPk = lastPool.Pk
	}
	txo, err := pi.composeTx(signerPk, sender, pi.chainID, blockHeight)
	if err != nil {
		return common.Hash{}, err
	}
	if nonce != nil {
		txo.Nonce = nonce
	}
	pi.logger.Info().Msgf("we have get the signature from tss module")
	txo.NoSend = true
	tokenInstance, err := generated.NewToken(common.HexToAddress(tokenAddr), pi.EthClient)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to generate token instance for %v while processing outbound tx", tokenAddr)
	}
	readyTx, err := tokenInstance.Transfer(txo, receiver, amount)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to send the token to the address %v with amount %v", receiver, amount.String())
		return common.Hash{}, err
	}

	ctxSend, cancelSend := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancelSend()

	if nonce != nil {
		err = pi.waitToSend(sender, nonce.Uint64())
		if err != nil {
			return readyTx.Hash(), err
		}
	}
	err = pi.sendTransactionWithLock(ctxSend, readyTx)
	if err != nil {
		//we reset the ethcliet
		pi.logger.Error().Err(err).Msgf("we fail to send the outbound ERC20 tx, have reset the eth client")
	}
	return readyTx.Hash(), err
}

// ProcessOutBound send the money to public chain
func (pi *Instance) ProcessOutBound(toAddr, fromAddr common.Address, tokenAddr string, amount *big.Int, blockHeight int64, nonce uint64) (string, error) {
	pi.logger.Info().Msgf(">>>>from addr %v to addr %v with amount %v of %v\n", fromAddr, toAddr, sdk.NewDecFromBigIntWithPrec(amount, 18), tokenAddr)
	var txHash common.Hash
	var err error
	if tokenAddr == config.NativeSign {
		txHash, _, err = pi.SendNativeToken("", fromAddr, toAddr, amount, blockHeight, new(big.Int).SetUint64(nonce))
	} else {
		txHash, err = pi.SendToken("", fromAddr, toAddr, amount, blockHeight, new(big.Int).SetUint64(nonce), tokenAddr)
	}
	if err != nil {
		if err.Error() == "already known" {
			pi.logger.Warn().Msgf("the tx has been submitted by others")
			return txHash.Hex(), nil
		}
		pi.logger.Error().Err(err).Msgf("fail to send the token with err %v", err)
		return txHash.Hex(), err
	}

	tick := html.UnescapeString("&#" + "128228" + ";")
	pi.logger.Info().Msgf("%v we have done the outbound tx %v", tick, txHash)
	return txHash.Hex(), nil
}

func (pi *Instance) tssSign(msg []byte, pk string, blockHeight int64) ([]byte, error) {
	encodedMsg := base64.StdEncoding.EncodeToString(msg)
	resp, err := pi.tssServer.KeySign(pk, []string{encodedMsg}, blockHeight, nil, "0.15.0")
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to run the keysign")
		return nil, err
	}

	if resp.Status != common3.Success {
		pi.logger.Error().Err(err).Msg("fail to generate the signature")
		// todo we need to handle the blame
		return nil, err
	}
	if len(resp.Signatures) != 1 {
		pi.logger.Error().Msgf("we should only have 1 signature")
		return nil, errors.New("more than 1 signature received")
	}
	signature, err := misc.SerializeSig(&resp.Signatures[0], true)
	if err != nil {
		pi.logger.Error().Msgf("fail to encode the signature")
		return nil, err
	}
	return signature, nil
}

func (pi *Instance) composeTx(signerPk string, sender common.Address, chainID *big.Int, blockHeight int64) (*bind.TransactOpts, error) {
	if chainID == nil {
		return nil, bind.ErrNoChainID
	}
	signer := types.LatestSignerForChainID(chainID)
	return &bind.TransactOpts{
		From: sender,
		Signer: func(address common.Address, tx *types.Transaction) (*types.Transaction, error) {
			if address != sender {
				return nil, errors.New("the address is different from the sender")
			}
			msg := signer.Hash(tx).Bytes()
			signature, err := pi.tssSign(msg, signerPk, blockHeight)
			if err != nil || len(signature) != 65 {
				return nil, errors.New("fail to sign the tx")
			}
			return tx.WithSignature(signer, signature)
		},
		Context: context.Background(),
	}, nil
}
