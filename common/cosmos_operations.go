package common

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosTx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/gogo/protobuf/grpc"
	types2 "github.com/tendermint/tendermint/proto/tendermint/types"
	grpc2 "google.golang.org/grpc"
)

const grpcTimeout = time.Second * 30

// QueryAccount get the current sender Account info
func QueryAccount(grpcClient grpc.ClientConn, addr, grpcAddr string) (types.AccountI, error) {
	var err error
	if grpcClient == nil {
		grpcClie2, err := grpc2.Dial(grpcAddr, grpc2.WithInsecure())
		if err != nil {
			return nil, err
		}
		defer grpcClie2.Close()
		grpcClient = grpcClie2
	}

	accQuery := types.NewQueryClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	accResp, err := accQuery.Account(ctx, &types.QueryAccountRequest{Address: addr})
	if err != nil {
		return nil, err
	}

	encCfg := MakeEncodingConfig()
	var acc types.AccountI
	if err := encCfg.InterfaceRegistry.UnpackAny(accResp.Account, &acc); err != nil {
		return nil, err
	}
	return acc, nil
}

// QueryBalance get the current sender Account info
func QueryBalance(grpcClient grpc.ClientConn, addr, grpcAddr string, denom string) (*sdk.Coin, error) {
	var err error
	if grpcClient == nil {
		grpcClie2, err := grpc2.Dial(grpcAddr, grpc2.WithInsecure())
		if err != nil {
			return nil, err
		}
		defer grpcClie2.Close()
		grpcClient = grpcClie2
	}

	accQuery := banktypes.NewQueryClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	atomBalanceResp, err := accQuery.Balance(ctx, &banktypes.QueryBalanceRequest{Address: addr, Denom: denom})
	if err != nil {
		return nil, err
	}
	return atomBalanceResp.Balance, nil
}

// GetLastBlockHeight get the last height of the joltify chain
func GetLastBlockHeight(grpcClient grpc.ClientConn) (int64, error) {
	ts := tmservice.NewServiceClient(grpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	resp, err := ts.GetLatestBlock(ctx, &tmservice.GetLatestBlockRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Block.Header.Height, nil
}

// GetGivenTx get the give tx with the txHash
func GetGivenTx(grpcClient grpc.ClientConn, txHash []byte) (*cosTx.GetTxResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	txClient := cosTx.NewServiceClient(grpcClient)
	txquery := cosTx.GetTxRequest{Hash: hex.EncodeToString(txHash)}
	t, err := txClient.GetTx(ctx, &txquery)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetBlockByHeight get the block from joltify chain based on provided height
func GetBlockByHeight(grpcClient grpc.ClientConn, height int64) (*types2.Block, error) {
	ts := tmservice.NewServiceClient(grpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	req := tmservice.GetBlockByHeightRequest{
		Height: height,
	}
	resp, err := ts.GetBlockByHeight(ctx, &req)
	if err != nil {
		return nil, err
	}
	return resp.Block, nil
}
