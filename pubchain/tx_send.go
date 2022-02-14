package pubchain

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"math/big"
	"sync"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	common3 "github.com/joltgeorge/tss/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

// SendToken sends the token to the public chain
func (pi *PubChainInstance) SendToken(signerPk string, sender, receiver common.Address, amount *big.Int, blockHeight int64) (string, error) {
	tokenInstance := pi.tokenInstance
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	chainID, err := pi.EthClient.NetworkID(ctx)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to get the chain ID")
		return "", err
	}
	if signerPk == "" {
		lastPool := pi.GetPool()[1]
		signerPk = lastPool.Pk
	}
	txo, err := pi.composeTx(signerPk, sender, chainID, blockHeight)
	if err != nil {
		return "", err
	}

	ret, err := tokenInstance.Transfer(txo, receiver, amount)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to send the token to the address %v with amount %v", receiver, amount.String())
		return "", err
	}
	tick := html.UnescapeString("&#" + "128228" + ";")
	pi.logger.Info().Msgf("%v we have done the outbound tx %v", tick, ret.Hash().Hex())
	return ret.Hash().Hex(), nil
}

// ProcessOutBound send the money to public chain
func (pi *PubChainInstance) ProcessOutBound(toAddr, fromAddr common.Address, amount *big.Int, blockHeight int64) (string, error) {
	pi.logger.Info().Msgf(">>>>from addr %v to addr %v with amount %v\n", fromAddr, toAddr, sdk.NewDecFromBigIntWithPrec(amount, 18))
	txHash, err := pi.SendToken("", fromAddr, toAddr, amount, blockHeight)
	if err != nil {
		if err.Error() == "already known" {
			pi.logger.Warn().Msgf("the tx has been submitted by others")
			return txHash, nil
		}
		pi.logger.Error().Err(err).Msgf("fail to send the token with err %v", err)
		return "", err
	}
	return txHash, nil
}

// StartSubscription start the subscription of the token
func (pi *PubChainInstance) StartSubscription(ctx context.Context, wg *sync.WaitGroup) (chan *types.Header, error) {
	blockEvent := make(chan *types.Header)
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

func (pi *PubChainInstance) composeTx(signerPk string, sender common.Address, chainID *big.Int, blockHeight int64) (*bind.TransactOpts, error) {
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
			encodedMsg := base64.StdEncoding.EncodeToString(msg)
			resp, err := pi.tssServer.KeySign(signerPk, []string{encodedMsg}, blockHeight, nil, "0.15.0")
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

			return tx.WithSignature(signer, signature)
		},
		Context: context.Background(),
	}, nil
}
