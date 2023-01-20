package cosbridge

import (
	"context"

	zlog "github.com/rs/zerolog/log"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"

	grpc1 "github.com/gogo/protobuf/grpc"
	"google.golang.org/grpc"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	sdk "github.com/cosmos/cosmos-sdk/types"
	pricefeedtypes "github.com/joltify-finance/joltify_lending/x/third_party/pricefeed/types"
	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
)

// queryLastValidatorSet get the last two validator sets
func queryLastValidatorSet(grpcClient grpc1.ClientConn) ([]*vaulttypes.PoolInfo, error) {
	ts := vaulttypes.NewQueryClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	req := vaulttypes.QueryLatestPoolRequest{}
	resp, err := ts.GetLastPool(ctx, &req)
	if err != nil {
		return nil, err
	}

	return resp.Pools, nil
}

func QueryTokenPrice(grpcClient grpc1.ClientConn, grpcAddr string, denom string) (sdk.Dec, error) {
	if grpcClient == nil {
		grpcClient2, err := grpc.Dial(grpcAddr, grpc.WithInsecure())
		if err != nil {
			zlog.Logger.Error().Err(err).Msgf("fail to dial the grpc end-point")
			return sdk.Dec{}, err
		}
		defer grpcClient2.Close()
		grpcClient = grpcClient2
	}

	qs := pricefeedtypes.NewQueryClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	var marketID string
	if denom == "ujolt" {
		marketID = "jolt:usd"
	} else {
		marketID = denom[1:] + ":usd"
	}
	req := pricefeedtypes.QueryPriceRequest{MarketId: marketID}

	result, err := qs.Price(ctx, &req)
	if err != nil {
		return sdk.Dec{}, err
	}
	return result.Price.Price, nil
}

// QueryTipValidator get the validator set of the tip of the current pub_chain
func QueryTipValidator(grpcClient grpc1.ClientConn) (int64, []*tmservice.Validator, error) {
	ts := tmservice.NewServiceClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	resp, err := ts.GetLatestValidatorSet(ctx, &tmservice.GetLatestValidatorSetRequest{})
	if err != nil {
		return 0, nil, err
	}

	return resp.BlockHeight, resp.Validators, nil
}

func outboundAdjust(amount sdk.Int, decimals int, precision int) sdk.Int {
	delta := decimals - precision
	if delta != 0 {
		adjustedTokenAmount := bcommon.AdjustInt(amount, int64(delta))
		return adjustedTokenAmount
	}
	return amount
}
