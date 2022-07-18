package pubchain

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// UpdateSubscription start the subscription of the token
func (pi *Instance) UpdateSubscription(ctx context.Context) error {
	blockEvent := make(chan *types.Header, sbchannelsize)
	pi.ethClientLocker.Lock()
	defer pi.ethClientLocker.Unlock()
	handler, err := pi.EthClient.SubscribeNewHead(ctx, blockEvent)
	if err != nil {
		fmt.Printf("fail to subscribe the block event with err %v\n", err)
		return err
	}
	if len(pi.SubChannelNow) > 0 {
		for {
			select {
			case b := <-pi.SubChannelNow:
				pi.ChannelQueue <- b
			default:
				break
			}
		}
	}
	pi.SubChannelNow = blockEvent
	pi.SubHandler = handler
	return nil
}

// StartSubscription start the subscription of the token
func (pi *Instance) StartSubscription(ctx context.Context, wg *sync.WaitGroup) error {
	blockEvent := make(chan *types.Header, sbchannelsize)
	handler, err := pi.EthClient.SubscribeNewHead(ctx, blockEvent)
	if err != nil {
		fmt.Printf("fail to subscribe the block event with err %v\n", err)
		return err
	}
	pi.ChannelQueue = blockEvent
	pi.SubHandler = handler

	go func() {
		<-ctx.Done()
		pi.SubHandler.Unsubscribe()
		pi.logger.Info().Msgf("shutdown the public pub_chain subscription channel")
		wg.Done()
	}()
	return nil
}

func (pi *Instance) RetryPubChain() {
	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Second*10), 3)
	op := func() error {
		ethClient, err := ethclient.Dial(pi.configAddr)
		if err != nil {
			pi.logger.Error().Err(err).Msg("fail to dial the websocket")
			return err
		}
		pi.logger.Warn().Msgf("we renewed the ethclient")
		pi.renewEthClientWithLock(ethClient)
		return nil
	}
	err := backoff.Retry(op, bf)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("we fail to reconnect the pubchain interface with retries")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	err = pi.UpdateSubscription(ctx)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("we fail to update the pubchain subscription")
	}

}
