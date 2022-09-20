package pubchain

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"math/big"
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

// SendNativeTokenBatch sends the native token to the public chain
func (pi *Instance) SendNativeTokenBatch(index int, sender, receiver common.Address, amount *big.Int, nonce *big.Int, tssReqChan chan *TssReq, tssRespChan chan map[string][]byte) (common.Hash, bool, error) {
	totalFee, gasPrice, adjGas, _, err := pi.GetFeeLimitWithLock()
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to get the suggested gas price")
		return common.Hash{}, false, err
	}

	txo, err := pi.composeTxBatch(index, sender, pi.chainID, tssReqChan, tssRespChan)
	if err != nil {
		return common.Hash{}, false, err
	}
	if nonce != nil {
		txo.Nonce = nonce
	}
	sendAmount := new(big.Int).Sub(amount, totalFee)
	pi.logger.Info().Msgf("we send %v with paid fee %v\n", amount, totalFee)
	txo.Value = sendAmount

	var data []byte
	tx := types.NewTx(&types.LegacyTx{Nonce: nonce.Uint64(), GasPrice: gasPrice, Gas: uint64(adjGas), To: &receiver, Value: sendAmount, Data: data})

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

// SendNativeTokenForMoveFund sends the native token to the public chain
func (pi *Instance) SendNativeTokenForMoveFund(signerPk string, sender, receiver common.Address, amount *big.Int, nonce *big.Int) (common.Hash, bool, error) {
	_, price, _, gas, err := pi.GetFeeLimitWithLock()
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to get the suggested gas price")
		return common.Hash{}, false, err
	}

	adjGas := int64(float32(gas) * config.MoveFundPubChainGASFEERATIO)
	fee := new(big.Int).Mul(price, big.NewInt(adjGas))
	// this statement is useful in
	if amount.Cmp(fee) != 1 {
		return common.Hash{}, true, nil
	}

	if signerPk == "" {
		lastPool := pi.GetPool()[1]
		signerPk = lastPool.Pk
	}

	latest, err := pi.GetBlockByNumberWithLock(nil)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to get the latest height")
		return common.Hash{}, false, err
	}
	blockHeight := int64(latest.NumberU64()) / ROUNDBLOCK
	tick := html.UnescapeString("&#" + "128296" + ";")
	pi.logger.Info().Msgf(">>>>>>%v we build tss at height %v>>>>>>>\n", tick, blockHeight)
	txo, err := pi.composeTx(signerPk, sender, pi.chainID, blockHeight)
	if err != nil {
		return common.Hash{}, false, err
	}
	if nonce != nil {
		txo.Nonce = nonce
	}
	pi.logger.Info().Msgf("we have get the signature for native token")
	sendAmount := new(big.Int).Sub(amount, fee)
	txo.Value = sendAmount

	var data []byte
	tx := types.NewTx(&types.LegacyTx{Nonce: nonce.Uint64(), GasPrice: price, Gas: uint64(adjGas), To: &receiver, Value: sendAmount, Data: data})

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

// SendTokenBatch sends the token to the public chain
func (pi *Instance) SendTokenBatch(index int, sender, receiver common.Address, amount *big.Int, nonce *big.Int, tokenAddr string, tssReqChan chan *TssReq, tssRespChan chan map[string][]byte) (common.Hash, error) {
	txo, err := pi.composeTxBatch(index, sender, pi.chainID, tssReqChan, tssRespChan)
	if err != nil {
		return common.Hash{}, err
	}
	if nonce != nil {
		txo.Nonce = nonce
	}
	txo.NoSend = true
	tokenInstance, err := generated.NewToken(common.HexToAddress(tokenAddr), pi.EthClient)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to generate token instance for %v while processing outbound tx", tokenAddr)
		return common.Hash{}, err
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
		// we reset the ethcliet
		pi.logger.Error().Err(err).Msgf("we fail to send the outbound ERC20 tx, have reset the eth client")
	}
	return readyTx.Hash(), err
}

// SendToken sends the token to the public chain
func (pi *Instance) SendToken(signerPk string, sender, receiver common.Address, amount *big.Int, nonce *big.Int, tokenAddr string) (common.Hash, error) {
	if signerPk == "" {
		lastPool := pi.GetPool()[1]
		signerPk = lastPool.Pk
	}

	latest, err := pi.GetBlockByNumberWithLock(nil)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to get the latest height")
		return common.Hash{}, err
	}

	blockHeight := int64(latest.NumberU64()) / ROUNDBLOCK

	tick := html.UnescapeString("&#" + "128296" + ";")
	pi.logger.Info().Msgf(">>>>>>%v we build tss at height %v>>>>>>>\n", tick, blockHeight)
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
		return common.Hash{}, err
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
		// we reset the ethcliet
		pi.logger.Error().Err(err).Msgf("we fail to send the outbound ERC20 tx, have reset the eth client")
	}
	return readyTx.Hash(), err
}

// ProcessOutBound send the money to public chain
func (pi *Instance) ProcessOutBound(index int, toAddr, fromAddr common.Address, tokenAddr string, amount *big.Int, nonce uint64, tssReqChan chan *TssReq, tssRespChan chan map[string][]byte) (string, error) {
	pi.logger.Info().Msgf(">>>>from addr %v to addr %v with amount %v of %v\n", fromAddr, toAddr, sdk.NewDecFromBigIntWithPrec(amount, 18), tokenAddr)
	var txHash common.Hash
	var err error
	if tokenAddr == config.NativeSign {
		txHash, _, err = pi.SendNativeTokenBatch(index, fromAddr, toAddr, amount, new(big.Int).SetUint64(nonce), tssReqChan, tssRespChan)
		tssReqChan <- &TssReq{Index: index, Data: []byte("done")}
	} else {
		txHash, err = pi.SendTokenBatch(index, fromAddr, toAddr, amount, new(big.Int).SetUint64(nonce), tokenAddr, tssReqChan, tssRespChan)
		tssReqChan <- &TssReq{Index: index, Data: []byte("done")}
	}
	if err != nil {
		if err.Error() == "already known" {
			pi.logger.Warn().Msgf("the tx has been submitted by others")
			return txHash.Hex(), nil
		}
		pi.logger.Error().Err(err).Msgf("fail to send the token with err %v", err)
		return txHash.Hex(), err
	}

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

func (pi *Instance) TssSignBatch(msgs [][]byte, pk string, blockHeight int64) (map[string][]byte, error) {
	var encodedMSgs []string
	for _, el := range msgs {
		encodedMsg := base64.StdEncoding.EncodeToString(el)
		encodedMSgs = append(encodedMSgs, encodedMsg)
	}
	resp, err := pi.tssServer.KeySign(pk, encodedMSgs, blockHeight, nil, "0.15.0")
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to run the keysign")
		return nil, err
	}

	if resp.Status != common3.Success {
		pi.logger.Error().Err(err).Msg("fail to generate the signature")
		// todo we need to handle the blame
		return nil, err
	}
	signatureMap := make(map[string][]byte)
	for _, eachSign := range resp.Signatures {
		s := eachSign
		signature, err := misc.SerializeSig(&s, true)
		if err != nil {
			pi.logger.Error().Msgf("fail to encode the signature")
			continue
		}
		signatureMap[eachSign.Msg] = signature
	}
	return signatureMap, nil
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

func (pi *Instance) composeTxBatch(index int, sender common.Address, chainID *big.Int, tssReqChan chan *TssReq, tssRespChan chan map[string][]byte) (*bind.TransactOpts, error) {
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
			tssReqChan <- &TssReq{Data: msg, Index: index}
			signature := <-tssRespChan
			if signature == nil {
				return nil, errors.New("fail to sign the tx")
			}

			expected := base64.StdEncoding.EncodeToString(msg)
			sig, ok := signature[expected]
			if !ok {
				return nil, errors.New("fail to sign the tx")
			}
			return tx.WithSignature(signer, sig)
		},
		Context: context.Background(),
	}, nil
}
