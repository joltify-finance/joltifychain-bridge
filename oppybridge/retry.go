package oppybridge

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	tmclienthttp "github.com/tendermint/tendermint/rpc/client/http"
	"google.golang.org/grpc"
)

// AddSubscribe add the subscribe to the chain
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
		quite := false
		for {
			select {
			case b := <-oc.CurrentNewBlockChan:
				oc.ChannelQueueNewBlock <- b
			default:
				quite = true
			}
			if quite {
				break
			}
		}
	}

	if len(oc.CurrentNewValidator) > 0 {
		for {
			quite := false
			select {
			case b := <-oc.CurrentNewValidator:
				oc.ChannelQueueValidator <- b
			default:
				quite = true
			}
			if quite {
				break
			}
		}
	}

	oc.CurrentNewBlockChan = newOppyBlock
	oc.CurrentNewValidator = validator
	return nil
}

func (oc *OppyChainInstance) RetryOppyChain() error {
	_, err1 := GetLastBlockHeight(oc.GrpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	_, err2 := oc.WsClient.Status(ctx)
	if err1 == nil && err2 == nil {
		oc.logger.Info().Msgf("all good,we do not need to reset")
		return nil
	}

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

		err = client.Start()
		if err != nil {
			return err
		}

		oc.retryLock.Lock()
		defer oc.retryLock.Unlock()
		oc.logger.Warn().Msgf("we renewed the oppy client")

		// we stop the old connection though it may broken
		// if we stop the channle directly, we do not need to unsubscribe it here
		// ctx,cancel:=context.WithTimeout(context.Background(),time.Second*3)
		// err=oc.WsClient.UnsubscribeAll(ctx,"oppyBridgeChurn")
		// if err!=nil{
		//	oc.logger.Error().Err(err).Msgf("we fail to unsubscribe")
		// }
		//
		// err=oc.WsClient.UnsubscribeAll(ctx,"oppyBridgeChurn")
		// if err!=nil{
		//	oc.logger.Error().Err(err).Msgf("we fail to unsubscribe")
		// }
		//

		err = oc.WsClient.Stop()
		if err != nil {
			oc.logger.Error().Err(err).Msgf("we fail to stop the previous WS client")
		}

		conn := oc.GrpcClient.(*grpc.ClientConn)
		err = conn.Close()
		if err != nil {
			oc.logger.Error().Err(err).Msgf("we fail to stop the grpc connection")
		}

		oc.GrpcClient = grpcClient
		oc.WsClient = client
		ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel2()
		err = oc.UpdateSubscribe(ctx2)
		return err
	}
	err := backoff.Retry(op, bf)
	if err != nil {
		oc.logger.Error().Err(err).Msgf("we fail to reconnect the pubchain interface with retries")
	}
	return err
}
