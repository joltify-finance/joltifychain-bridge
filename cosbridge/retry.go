package cosbridge

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	tmclienthttp "github.com/tendermint/tendermint/rpc/client/http"
	"google.golang.org/grpc"
)

// AddSubscribe add the subscribe to the chain
func (jc *JoltChainInstance) AddSubscribe(ctx context.Context) error {
	var err error
	query := "complete_churn.churn = 'oppy_churn'"
	jc.CurrentNewValidator, err = jc.WsClient.Subscribe(ctx, "oppyBridgeChurn", query, capacity)
	if err != nil {
		jc.logger.Error().Err(err).Msgf("Failed to subscribe to query with error %v", err)
		return err
	}

	query = "tm.event = 'NewBlock'"
	jc.CurrentNewBlockChan, err = jc.WsClient.Subscribe(ctx, "oppyBridgeNewBlock", query, capacity)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return err
	}
	return nil
}

func (jc *JoltChainInstance) ProcessNewBlockChainMoreThanOne() {
	if len(jc.CurrentNewValidator) > 0 {
		for {
			quite := false
			select {
			case b := <-jc.CurrentNewValidator:
				jc.ChannelQueueValidator <- b
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
func (jc *JoltChainInstance) UpdateSubscribe(ctx context.Context) error {
	query := "complete_churn.churn = 'oppy_churn'"
	validator, err := jc.WsClient.Subscribe(ctx, "oppyBridgeChurn", query, capacity)
	if err != nil {
		jc.logger.Error().Err(err).Msgf("Failed to subscribe to query with error %v", err)
		return err
	}

	query = "tm.event = 'NewBlock'"
	newOppyBlock, err := jc.WsClient.Subscribe(ctx, "oppyBridgeNewBlock", query, capacity)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return err
	}

	if len(jc.CurrentNewBlockChan) > 0 {
		quite := false
		for {
			select {
			case b := <-jc.CurrentNewBlockChan:
				jc.ChannelQueueNewBlock <- b
			default:
				quite = true
			}
			if quite {
				break
			}
		}
	}
	jc.ProcessNewBlockChainMoreThanOne()

	jc.CurrentNewBlockChan = newOppyBlock
	jc.CurrentNewValidator = validator
	return nil
}

func (jc *JoltChainInstance) RetryOppyChain() error {
	_, err1 := GetLastBlockHeight(jc.GrpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	_, err2 := jc.WsClient.Status(ctx)
	if err1 == nil && err2 == nil {
		jc.logger.Info().Msgf("all good,we do not need to reset")
		return nil
	}

	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Second*10), 3)
	op := func() error {
		grpcClient, err := grpc.Dial(jc.grpcAddr, grpc.WithInsecure())
		if err != nil {
			return err
		}
		client, err := tmclienthttp.New(jc.httpAddr, "/websocket")
		if err != nil {
			return err
		}

		err = client.Start()
		if err != nil {
			return err
		}

		jc.retryLock.Lock()
		defer jc.retryLock.Unlock()
		jc.logger.Warn().Msgf("we renewed the joltify client")

		err = jc.WsClient.Stop()
		if err != nil {
			jc.logger.Error().Err(err).Msgf("we fail to stop the previous WS client")
		}

		conn := jc.GrpcClient.(*grpc.ClientConn)
		err = conn.Close()
		if err != nil {
			jc.logger.Error().Err(err).Msgf("we fail to stop the grpc connection")
		}

		jc.GrpcClient = grpcClient
		jc.WsClient = client
		ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel2()
		err = jc.UpdateSubscribe(ctx2)
		return err
	}
	err := backoff.Retry(op, bf)
	if err != nil {
		jc.logger.Error().Err(err).Msgf("we fail to reconnect the pubchain interface with retries")
	}
	return err
}
