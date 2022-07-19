package oppybridge

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	tmclienthttp "github.com/tendermint/tendermint/rpc/client/http"
	"google.golang.org/grpc"
)

// AddSubscribe add the subscirbe to the chain
func (oc *OppyChainInstance) AddSubscribe(ctx context.Context) error {

	var err error
	query := "complete_churn.churn = 'oppy_churn'"
	oc.CurrentNewValidator, err = oc.WsClient.Subscribe(ctx, "oppyBridgeChurn", query, capacity)
	if err != nil {
		oc.logger.Error().Err(err).Msgf("Failed to subscribe to query with error %v", err)
		return err
	}

	query = "tm.event = 'NewBlock'"
	oc.CurrentNewBlockChan, err = oc.WsClient.Subscribe(ctx, "oppyBridgeNewBlock", query, capacity)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return err
	}
	return nil
}

// UpdateSubscribe add the subscribe to the chain
func (oc *OppyChainInstance) UpdateSubscribe(ctx context.Context) error {
	query := "complete_churn.churn = 'oppy_churn'"
	validator, err := oc.WsClient.Subscribe(ctx, "oppyBridgeChurn", query, capacity)
	if err != nil {
		oc.logger.Error().Err(err).Msgf("Failed to subscribe to query with error %v", err)
		return err
	}

	query = "tm.event = 'NewBlock'"
	newOppyBlock, err := oc.WsClient.Subscribe(ctx, "oppyBridgeNewBlock", query, capacity)
	if err != nil {
		fmt.Printf("fail to start the subscription")
		return err
	}

	if len(oc.CurrentNewBlockChan) > 0 {
		for {
			select {
			case b := <-oc.CurrentNewValidator:
				oc.ChannelQueueNewBlock <- b
			default:
				break
			}
		}
	}

	if len(oc.CurrentNewValidator) > 0 {
		for {
			select {
			case b := <-oc.CurrentNewValidator:
				oc.ChannelQueueValidator <- b
			default:
				break
			}
		}
	}

	oc.CurrentNewBlockChan = newOppyBlock
	oc.CurrentNewValidator = validator
	return nil
}

func (oc *OppyChainInstance) RetryOppyChain() error {
	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Second*10), 3)
	op := func() error {

		grpcClient, err := grpc.Dial(oc.grpcAddr, grpc.WithInsecure())
		if err != nil {
			return err
		}
		client, err := tmclienthttp.New(oc.httpAddr, "/websocket")
		if err != nil {
			return err
		}
		oc.retryLock.Lock()
		defer oc.retryLock.Unlock()
		oc.logger.Warn().Msgf("we renewed the oppy client")
		oc.GrpcClient = grpcClient
		oc.WsClient = client
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel()
		oc.UpdateSubscribe(ctx)

		return nil
	}
	err := backoff.Retry(op, bf)
	if err != nil {
		oc.logger.Error().Err(err).Msgf("we fail to reconnect the pubchain interface with retries")
	}
	return err
}
