package pubchain

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	etypes "github.com/ethereum/go-ethereum/core/types"
)

//StartSubscription start the subscription of the token
func (ci *PubChainInstance) StartSubscription(ctx context.Context, wg *sync.WaitGroup) (chan *etypes.Header, error) {

	ctxWatch, _ := context.WithTimeout(context.Background(), chainQueryTimeout)
	watchOpt := bind.WatchOpts{
		Context: ctxWatch,
	}
	pools := ci.GetPool()
	var watchList []common.Address
	for _, el := range pools {
		if len(el) != 0 {
			watchList = append(watchList, el)
		}
	}
	sbEvent, err := ci.tokenSb.tokenInstance.WatchTransfer(&watchOpt, ci.tokenSb.sb, nil, watchList)
	if err != nil {
		ci.logger.Error().Err(err).Msgf("fail to setup watcher")
		return nil, errors.New("fail to setup the watcher")
	}
	ci.tokenSb.UpdateSbEvent(sbEvent)

	blockEvent := make(chan *etypes.Header)
	blockSub, err := ci.EthClient.SubscribeNewHead(ctx, blockEvent)
	if err != nil {
		fmt.Printf("fail to subscribe the block event with err %v\n", err)
		return nil, err
	}

	go func() {
		<-ctx.Done()
		blockSub.Unsubscribe()
		ci.logger.Info().Msgf("shutdown the public pub_chain subscription channel")
		wg.Done()
	}()
	return blockEvent, nil
}

//UpdateSubscribe update the subscribed pool address
func (ci *PubChainInstance) UpdateSubscribe() error {
	//fixme ctx should be global parameter
	ctx, _ := context.WithTimeout(context.Background(), chainQueryTimeout)
	watchOpt := bind.WatchOpts{
		Context: ctx,
	}
	pools := ci.GetPool()
	var watchList []common.Address
	for _, el := range pools {
		if len(el) != 0 {
			watchList = append(watchList, el)
		}
	}
	//cancel the previous subscription
	sbEvent, err := ci.tokenSb.tokenInstance.WatchTransfer(&watchOpt, ci.tokenSb.sb, nil, watchList)
	if err != nil {
		ci.logger.Error().Err(err).Msg("fail to subscribe the event")
		return err
	}
	ci.tokenSb.UpdateSbEvent(sbEvent)
	ci.logger.Info().Msgf("we update the event to address %v\n", watchList)
	return nil
}

//GetSubChannel gets the subscription channel
func (ci *PubChainInstance) GetSubChannel() chan *TokenTransfer {
	return ci.tokenSb.sb
}
