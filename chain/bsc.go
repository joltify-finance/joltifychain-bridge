package chain

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
	tokenIns, err := NewToken(common.HexToAddress(ci.tokenAddr), ci.EthClient)
	if err != nil {
		ci.logger.Error().Err(err).Msg("fail to generate new token")
		return nil, errors.New("fail to get the new token")
	}
	ci.tokenSb.tokenInstance = tokenIns

	watchOpt := bind.WatchOpts{
		Context: context.Background(),
	}
	pools := ci.getPool()
	var watchList []common.Address
	for _, el := range pools {
		if el != "" {
			watchList = append(watchList, common.HexToAddress(el))
		}
	}
	sbEvent, err := ci.tokenSb.tokenInstance.WatchTransfer(&watchOpt, ci.tokenSb.sb, nil, watchList)
	if err != nil {
		ci.logger.Error().Err(err).Msgf("fail to setup watcher")
		return nil, errors.New("fail to setup the watcher")
	}
	ci.tokenSb.sbEvent = &sbEvent

	blockEvent := make(chan *etypes.Header)
	blockSub, err := ci.EthClient.SubscribeNewHead(ctx, blockEvent)
	if err != nil {
		fmt.Printf("fail to subscribe the block event with err %v\n", err)
		return nil, err
	}

	go func() {
		select {
		case <-ctx.Done():
			blockSub.Unsubscribe()
			ci.logger.Info().Msgf("shutdown the public chain subscription channel")
			wg.Done()
			return
		}
	}()
	return blockEvent, nil
}

//UpdateSubscribe update the subscribed pool address
func (ci *PubChainInstance) UpdateSubscribe(addrs []common.Address) error {
	ci.tokenSb.lock.Lock()
	defer ci.tokenSb.lock.Unlock()
	watchOpt := bind.WatchOpts{
		Context: context.Background(),
	}
	sink := make(chan *TokenTransfer)
	sbEvent, err := ci.tokenSb.tokenInstance.WatchTransfer(&watchOpt, sink, nil, addrs)
	ci.tokenSb.sbEvent = &sbEvent
	if err != nil {
		ci.logger.Error().Err(err).Msg("fail to subscribe the event")
		return err
	}
	return nil
}

//GetSubChannel gets the subscription channel
func (ci *PubChainInstance) GetSubChannel() chan *TokenTransfer {
	return ci.tokenSb.sb
}
