package pubchain

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"

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
func (pi *Instance) waitAndSend(poolAddress common.Address, targetNonce uint64) error {
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

// SendToken sends the token to the public chain
func (pi *Instance) SendToken(wg *sync.WaitGroup, signerPk string, sender, receiver common.Address, amount *big.Int, blockHeight int64, nonce *big.Int, tokenAddr string) (common.Hash, error) {
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
		err = pi.waitAndSend(sender, nonce.Uint64())
		if err != nil {
			return readyTx.Hash(), err
		}
	}
	err = pi.sendTransactionWithLock(ctxSend, readyTx)
	if err != nil {
		//we reset the ethcliet
		fmt.Printf("error of the ethclient is %v\n", err)
		wg.Add(1)
		go func() {
			defer wg.Done()
			bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Second*10), 3)

			op := func() error {
				ethClient, err := ethclient.Dial(pi.configAddr)
				if err != nil {
					pi.logger.Error().Err(err).Msg("fail to dial the websocket")
					return err
				}
				pi.renewEthClientWithLock(ethClient)
				return nil
			}

			err := backoff.Retry(op, bf)
			if err != nil {
				fmt.Printf("#########we fail all the retries#########")
				return
			}
		}()

	}

	return readyTx.Hash(), err
}

// ProcessOutBound send the money to public chain
func (pi *Instance) ProcessOutBound(wg *sync.WaitGroup, toAddr, fromAddr common.Address, tokenAddr string, amount *big.Int, blockHeight int64, nonce uint64) (string, error) {
	pi.logger.Info().Msgf(">>>>from addr %v to addr %v with amount %v of %v\n", fromAddr, toAddr, sdk.NewDecFromBigIntWithPrec(amount, 18), tokenAddr)
	txHash, err := pi.SendToken(wg, "", fromAddr, toAddr, amount, blockHeight, new(big.Int).SetUint64(nonce), tokenAddr)
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

// StartSubscription start the subscription of the token
func (pi *Instance) StartSubscription(ctx context.Context, wg *sync.WaitGroup) (chan *types.Header, error) {
	blockEvent := make(chan *types.Header, sbchannelsize)
	blockSub, err := pi.EthClient.SubscribeNewHead(ctx, blockEvent)
	if err != nil {
		fmt.Printf("fail to subscribe the block event with err %v\n", err)
		return nil, err
	}

	go func() {
		<-ctx.Done()
		blockSub.Unsubscribe()
		pi.logger.Info().Msgf("shutdown the public pub_chain subscription channel")
		wg.Done()
	}()
	return blockEvent, nil
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
