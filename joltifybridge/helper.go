package joltifybridge

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
	"google.golang.org/grpc"
)

// queryAccount get the current sender account info
func queryAccount(addr string, grpcClient *grpc.ClientConn) (authtypes.AccountI, error) {
	accQuery := authtypes.NewQueryClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	accResp, err := accQuery.Account(ctx, &authtypes.QueryAccountRequest{Address: addr})
	if err != nil {
		return nil, err
	}

	encCfg := MakeEncodingConfig()
	var acc authtypes.AccountI
	if err := encCfg.InterfaceRegistry.UnpackAny(accResp.Account, &acc); err != nil {
		return nil, err
	}
	return acc, nil
}

// queryLastValidatorSet get the last two validator sets
func queryLastValidatorSet(grpcClient *grpc.ClientConn) ([]*vaulttypes.PoolInfo, error) {
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

// QueryTipValidator get the validator set of the tip of the current pub_chain
func QueryTipValidator(grpcClient *grpc.ClientConn) (int64, []*tmservice.Validator, error) {
	ts := tmservice.NewServiceClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	resp, err := ts.GetLatestValidatorSet(ctx, &tmservice.GetLatestValidatorSetRequest{})
	if err != nil {
		return 0, nil, err
	}

	return resp.BlockHeight, resp.Validators, nil
}

func GetLastBlockHeight(grpcClient *grpc.ClientConn) (int64, error) {
	ts := tmservice.NewServiceClient(grpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	resp, err := ts.GetLatestBlock(ctx, &tmservice.GetLatestBlockRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Block.Header.Height, nil
}
