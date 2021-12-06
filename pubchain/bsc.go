package pubchain

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"sync"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	etypes "github.com/ethereum/go-ethereum/core/types"
)

// StartSubscription start the subscription of the token
func (pi *PubChainInstance) StartSubscription(ctx context.Context, wg *sync.WaitGroup) (chan *etypes.Header, error) {
	ctxWatch, _ := context.WithTimeout(context.Background(), chainQueryTimeout)
	watchOpt := bind.WatchOpts{
		Context: ctxWatch,
	}
	pools := pi.GetPool()
	var watchList []common.Address
	for _, el := range pools {
		if len(el) != 0 {
			watchList = append(watchList, el)
		}
	}
	sbEvent, err := pi.tokenSb.tokenInstance.WatchTransfer(&watchOpt, pi.tokenSb.sb, nil, watchList)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to setup watcher")
		return nil, errors.New("fail to setup the watcher")
	}
	pi.tokenSb.UpdateSbEvent(sbEvent)

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

// UpdateSubscribe update the subscribed pool address
func (pi *PubChainInstance) UpdateSubscribe() error {
	// fixme ctx should be global parameter
	ctx, _ := context.WithTimeout(context.Background(), chainQueryTimeout)
	watchOpt := bind.WatchOpts{
		Context: ctx,
	}
	pools := pi.GetPool()
	var watchList []common.Address
	for _, el := range pools {
		if len(el) != 0 {
			watchList = append(watchList, el)
		}
	}
	// cancel the previous subscription
	sbEvent, err := pi.tokenSb.tokenInstance.WatchTransfer(&watchOpt, pi.tokenSb.sb, nil, watchList)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to subscribe the event")
		return err
	}
	pi.tokenSb.UpdateSbEvent(sbEvent)
	pi.logger.Info().Msgf("we update the event to address %v\n", watchList)
	return nil
}

// GetSubChannel gets the subscription channel
func (pi *PubChainInstance) GetSubChannel() chan *TokenTransfer {
	return pi.tokenSb.sb
}

func (pi *PubChainInstance) composeTx(sender common.Address, chainID *big.Int, blockHeight int64) (*bind.TransactOpts, error) {
	if chainID == nil {
		return nil, bind.ErrNoChainID
	}
	cPk, err := sdk.GetPubKeyFromBech32(sdk.Bech32PubKeyTypeAccPub, pk)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to get the public key from bech32 format")
		return nil, err
	}

	lastPool := pi.GetPool()[0]
	lastPool.Bytes()
	signer := etypes.LatestSignerForChainID(chainID)
	return &bind.TransactOpts{
		From: sender,
		Signer: func(address common.Address, tx *etypes.Transaction) (*etypes.Transaction, error) {
			msg := signer.Hash(tx).Bytes()
			encodedMsg := base64.StdEncoding.EncodeToString(msg)
			resp, err := pi.tssServer.KeySign(cPk.String(), []string{encodedMsg}, blockHeight, nil, "0.15.0")
			if err != nil {
				pi.logger.Error().Err(err).Msg("fail to run the keysign")
				return nil, err
			}

			return tx.WithSignature(signer, signature)
		},
		Context: context.Background(),
	}, nil
}
