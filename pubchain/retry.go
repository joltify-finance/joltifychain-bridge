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
func (c *Erc20ChainInfo) UpdateSubscription(ctx context.Context) error {
	blockEvent := make(chan *types.Header, sbchannelsize)
	handler, err := c.Client.SubscribeNewHead(ctx, blockEvent)
	if err != nil {
		fmt.Printf("fail to subscribe the block event with err %v\n", err)
		return err
	}
	if len(c.SubChannelNow) > 0 {
		quite := false
		for {
			select {
			case b := <-c.SubChannelNow:
				bHead := BlockHead{
					Head:      b,
					ChainType: c.ChainType,
				}
				c.ChannelQueue <- &bHead
			default:
				quite = true
			}
			if quite {
				break
			}
		}
	}
	c.SubChannelNow = blockEvent
	c.SubHandler = handler
	return nil
}

// StartSubscription start the subscription of the token
func (c *Erc20ChainInfo) StartSubscription(ctx context.Context, wg *sync.WaitGroup) error {
	c.SubChannelNow = make(chan *types.Header, sbchannelsize)
	handler, err := c.Client.SubscribeNewHead(ctx, c.SubChannelNow)
	if err != nil {
		fmt.Printf("fail to subscribe the block event with err %v\n", err)
		return err
	}
	c.SubHandler = handler
	wg.Add(1)
	go func() {
		<-ctx.Done()
		c.SubHandler.Unsubscribe()
		c.logger.Info().Msgf("shutdown the public pub_chain subscription channel")
		wg.Done()
	}()
	return nil
}

func (c *Erc20ChainInfo) RetryPubChain() error {
	err := c.CheckChainHealthWithLock()
	if err == nil {
		c.logger.Info().Msgf("all good we do not need to reset")
		return nil
	}

	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Second*10), 3)
	op := func() error {
		ethClient, err := ethclient.Dial(c.WsAddr)
		if err != nil {
			c.logger.Error().Err(err).Msg("fail to dial the websocket")
			return err
		}
		err = c.renewEthClientWithLock(ethClient)
		return err
	}
	err = backoff.Retry(op, bf)
	if err != nil {
		c.logger.Error().Err(err).Msgf("we fail to reconnect the pubchain interface with retries")
		return err
	}

	c.logger.Warn().Msgf("we renewed the ethclient")
	return nil
}
