package joltifybridge

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	grpc1 "github.com/gogo/protobuf/grpc"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

// QueryAccount get the current sender account info
func QueryAccount(addr string, grpcClient grpc1.ClientConn) (authtypes.AccountI, error) {
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

// queryBalance get the current sender account info
func queryBalance(addr string, grpcClient grpc1.ClientConn) (sdk.Coins, error) {
	accQuery := banktypes.NewQueryClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	resp, err := accQuery.AllBalances(ctx, &banktypes.QueryAllBalancesRequest{Address: addr})
	if err != nil {
		return nil, err
	}
	return resp.Balances, nil
}

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

// queryLastValidatorSet get the last two validator sets
func queryGivenToeknIssueTx(grpcClient grpc1.ClientConn, index string) (*vaulttypes.IssueToken, error) {
	ts := vaulttypes.NewQueryClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	req := vaulttypes.QueryGetIssueTokenRequest{
		Index: index,
	}
	resp, err := ts.IssueToken(ctx, &req)
	if err != nil {
		return nil, err
	}

	return resp.IssueToken, nil
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

// GetLastBlockHeight get the last height of the joltify chain
func GetLastBlockHeight(grpcClient grpc1.ClientConn) (int64, error) {
	ts := tmservice.NewServiceClient(grpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	resp, err := ts.GetLatestBlock(ctx, &tmservice.GetLatestBlockRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Block.Header.Height, nil
}

func (jc *JoltifyChainInstance) composeAndSend(sendMsg sdk.Msg, accSeq, accNum uint64, signMsg *tssclient.TssSignigMsg) (bool, string, error) {
	gasWanted, err := jc.GasEstimation([]sdk.Msg{sendMsg}, accSeq, signMsg)
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to get the gas estimation")
		return false, "", err
	}
	txBuilder, err := jc.genSendTx([]sdk.Msg{sendMsg}, accSeq, accNum, gasWanted, signMsg)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to generate the tx")
		return false, "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	txBytes, err := jc.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to encode the tx")
		return false, "", err
	}

	ok, resp, err := jc.BroadcastTx(ctx, txBytes)
	return ok, resp, err
}
