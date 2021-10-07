package bridge

import (
	"context"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

const capacity = 10000

func (ic *InvChainBridge) AddSubscribe(ctx context.Context, query string) (<-chan ctypes.ResultEvent, error) {

	out, err := ic.wsClient.Subscribe(ctx, "test", query, capacity)
	if err != nil {
		ic.logger.Error().Msgf("Failed to subscribe to query", "err", err, "query", query)
		return nil, err
	}

	return out, nil
}
