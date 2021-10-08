package bridge

import (
	"context"
	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"google.golang.org/grpc"
)

func QueryAccount(addr string, grpcClient *grpc.ClientConn) (authtypes.AccountI, error) {
	accQuery := authtypes.NewQueryClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	accResp, err := accQuery.Account(ctx, &authtypes.QueryAccountRequest{Address: addr})
	if err != nil {
		return nil, err
	}

	//acc := accResp.Account.GetCachedValue().(authtypes.AccountI)
	encCfg := MakeEncodingConfig()
	var acc authtypes.AccountI
	if err := encCfg.InterfaceRegistry.UnpackAny(accResp.Account, &acc); err != nil {
		return nil, err
	}
	return acc, nil
}

func QueryHistoricalValidator(grpcClient *grpc.ClientConn) (int64, []*tmservice.Validator, error) {
	ts := tmservice.NewServiceClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	resp, err := ts.GetLatestValidatorSet(ctx, &tmservice.GetLatestValidatorSetRequest{})
	if err != nil {
		return 0, nil, err
	}

	//for i, el := range resp.Validators {
	//	fmt.Printf("%v========%v\n", i, el.GetAddress())
	//}
	return resp.BlockHeight, resp.Validators, nil
}

func GetLastBlock(grpcClient *grpc.ClientConn){

	ts := tmservice.NewServiceClient(grpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	resp, err := ts.GetLatestBlock(ctx, &tmservice.GetLatestBlockRequest{})
	if err != nil {
		return 0, nil, err
	}
	resp.


}
