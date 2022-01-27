package pubchain

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	etypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	tsscommon "github.com/joltgeorge/tss/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

// StartSubscription start the subscription of the token
func (pi *PubChainInstance) StartSubscription(ctx context.Context, wg *sync.WaitGroup) (chan *etypes.Header, error) {
	blockEvent := make(chan *etypes.Header)
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

func (pi *PubChainInstance) composeTx(sender common.Address, chainID *big.Int, blockHeight int64) (*bind.TransactOpts, error) {
	if chainID == nil {
		return nil, bind.ErrNoChainID
	}
	lastPool := pi.GetPool()[1]
	signer := etypes.LatestSignerForChainID(chainID)
	return &bind.TransactOpts{
		From: sender,
		Signer: func(address common.Address, tx *etypes.Transaction) (*etypes.Transaction, error) {
			if address != sender {
				return nil, errors.New("the address is different from the sender")
			}
			msg := signer.Hash(tx).Bytes()
			encodedMsg := base64.StdEncoding.EncodeToString(msg)
			resp, err := pi.tssServer.KeySign(lastPool.Pk, []string{encodedMsg}, blockHeight, nil, "0.15.0")
			if err != nil {
				pi.logger.Error().Err(err).Msg("fail to run the keysign")
				return nil, err
			}

			if resp.Status != tsscommon.Success {
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

			sigPublicKeyECDSA, err := crypto.SigToPub(msg, signature)
			if err != nil {
				panic(err)
			}

			addr := crypto.PubkeyToAddress(*sigPublicKeyECDSA)
			fmt.Printf(">>>>recovered address is %v", addr.String())
			return tx.WithSignature(signer, signature)
		},
		Context: context.Background(),
	}, nil
}
