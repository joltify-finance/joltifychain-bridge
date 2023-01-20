package cossubmit

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	tmclienthttp "github.com/tendermint/tendermint/rpc/client/http"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"google.golang.org/grpc"
)

// AddSubscribe add the subscribe to the chain
func (cs *CosHandler) AddSubscribe(ctx context.Context) error {
	var err error
	query := "complete_churn.churn = 'oppy_churn'"
	cs.CurrentNewValidator, err = cs.wsClient.Subscribe(ctx, "oppyBridgeChurn", query, capacity)
	if err != nil {
		cs.logger.Error().Err(err).Msgf("Failed to subscribe to query with error %v", err)
		return err
	}

	query = "tm.event = 'NewBlock'"
	cs.currentNewBlockChan, err = cs.wsClient.Subscribe(ctx, "oppyBridgeNewBlock", query, capacity)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return err
	}
	return nil
}

func (cs *CosHandler) ProcessNewBlockChainMoreThanOne() {
	if len(cs.CurrentNewValidator) > 0 {
		for {
			quite := false
			select {
			case b := <-cs.CurrentNewValidator:
				cs.ChannelQueueValidator <- b
			default:
				quite = true
			}
			if quite {
				break
			}
		}
	}
}

// UpdateSubscribe add the subscribe to the chain
func (cs *CosHandler) UpdateSubscribe(ctx context.Context) error {
	query := "complete_churn.churn = 'oppy_churn'"
	validator, err := cs.wsClient.Subscribe(ctx, "oppyBridgeChurn", query, capacity)
	if err != nil {
		cs.logger.Error().Err(err).Msgf("Failed to subscribe to query with error %v", err)
		return err
	}

	query = "tm.event = 'NewBlock'"
	newOppyBlock, err := cs.wsClient.Subscribe(ctx, "oppyBridgeNewBlock", query, capacity)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return err
	}

	if len(cs.currentNewBlockChan) > 0 {
		quite := false
		for {
			select {
			case b := <-cs.currentNewBlockChan:
				cs.ChannelQueueNewBlock <- b
			default:
				quite = true
			}
			if quite {
				break
			}
		}
	}
	cs.ProcessNewBlockChainMoreThanOne()

	cs.currentNewBlockChan = newOppyBlock
	cs.CurrentNewValidator = validator
	return nil
}

func (cs *CosHandler) RetryJoltifyChain(forceReset bool) error {
	_, err1 := common.GetLastBlockHeight(cs.GrpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	if !forceReset {
		_, err2 := cs.wsClient.Status(ctx)
		if err1 == nil && err2 == nil {
			cs.logger.Info().Msgf("all good,we do not need to reset")
			return nil
		}
	}

	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Second*10), 3)
	op := func() error {
		grpcClient, err := grpc.Dial(cs.GrpcAddr, grpc.WithInsecure())
		if err != nil {
			return err
		}
		client, err := tmclienthttp.New(cs.httpAddr, "/websocket")
		if err != nil {
			return err
		}

		err = client.Start()
		if err != nil {
			return err
		}

		cs.retryLock.Lock()
		defer cs.retryLock.Unlock()
		cs.logger.Warn().Msgf("we renewed the joltify client")

		err = cs.wsClient.Stop()
		if err != nil {
			cs.logger.Error().Err(err).Msgf("we fail to stop the previous WS client")
		}

		conn, ok := cs.GrpcClient.(*grpc.ClientConn)
		// for the testing, the clientCtx cannot be converted into grpc client
		if ok {
			err = conn.Close()
			if err != nil {
				cs.logger.Error().Err(err).Msgf("we fail to stop the grpc connection")
			}
		}

		cs.GrpcClient = grpcClient
		cs.wsClient = client
		ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel2()
		err = cs.UpdateSubscribe(ctx2)
		return err
	}
	err := backoff.Retry(op, bf)
	if err != nil {
		cs.logger.Error().Err(err).Msgf("we fail to reconnect the pubchain interface with retries")
	}
	return err
}
