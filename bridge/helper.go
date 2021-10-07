package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/types/rest"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	zlog "github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

type Info struct {
	Result struct {
		ValidatorInfo struct {
			Address string `json:"address"`
			PubKey  struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"pub_key"`
			VotingPower string `json:"voting_power"`
		} `json:"validator_info"`
	} `json:"result"`
}

func QueryAccount(addr string, grpcClient *grpc.ClientConn) (authtypes.AccountI, error) {
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

func QueryHistoricalValidator(grpcClient *grpc.ClientConn) ([]*tmservice.Validator, error) {
	ts := tmservice.NewServiceClient(grpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	for {
		result, err := ts.GetSyncing(ctx, &tmservice.GetSyncingRequest{})
		if err != nil {
			return nil, err
		}
		if !result.GetSyncing() {
			break
		}
		zlog.Logger.Info().Msg("the blockchain is not fully synced, please wait....")
		time.Sleep(time.Second * 5)
	}

	nodeInfo, err := ts.GetNodeInfo(ctx, &tmservice.GetNodeInfoRequest{})
	if err != nil {
		return nil, err
	}
	addr := "http://localhost:26657"
	restRes, err := rest.GetRequest(fmt.Sprintf("%s/status", addr))
	if err != nil {
		return nil, err
	}
	var info Info
	err = json.Unmarshal(restRes, &info)
	if err != nil {
		return nil, err
	}
	fmt.Printf(">>>>>local node address:%v\n", info.Result.ValidatorInfo.Address)

	fmt.Printf("==================%v======================\n", nodeInfo.DefaultNodeInfo.GetMoniker())

	resp, err := ts.GetLatestValidatorSet(ctx, &tmservice.GetLatestValidatorSetRequest{})
	if err != nil {
		return nil, err
	}

	for i, el := range resp.Validators {
		fmt.Printf("%v========%v\n", i, el.GetAddress())
	}
	return resp.Validators, nil
}
