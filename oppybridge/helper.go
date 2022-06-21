package oppybridge

import (
	"context"
	"errors"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"

	"github.com/cenkalti/backoff"
	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	grpc1 "github.com/gogo/protobuf/grpc"
	types "github.com/tendermint/tendermint/proto/tendermint/types"
	"gitlab.com/oppy-finance/oppy-bridge/tssclient"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
)

// queryAccount get the current sender account info
func queryAccount(addr string, grpcClient grpc1.ClientConn) (authtypes.AccountI, error) {
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

// GetLastBlockHeight get the last height of the oppy chain
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

// GetBlockByHeight get the block from oppy chain based on provided height
func GetBlockByHeight(grpcClient grpc1.ClientConn, height int64) (*types.Block, error) {
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

// CheckTxStatus check whether the tx has been done successfully
func (oc *OppyChainInstance) waitAndSend(poolAddress sdk.AccAddress, targetSeq uint64) error {
	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(submitBackoff), 40)

	alreadyPassed := false
	op := func() error {
		acc, err := queryAccount(poolAddress.String(), oc.grpcClient)
		if err != nil {
			oc.logger.Error().Err(err).Msgf("fail to query the account")
			return errors.New("invalid account query")
		}
		if acc.GetSequence() == targetSeq {
			return nil
		}
		if acc.GetSequence() > targetSeq {
			alreadyPassed = true
			return nil
		}
		return errors.New("not our round")
	}

	err := backoff.Retry(op, bf)
	if alreadyPassed {
		return errors.New("already passed")
	}
	return err
}

func (oc *OppyChainInstance) composeAndSend(operator keyring.Info, sendMsg sdk.Msg, accSeq, accNum uint64, signMsg *tssclient.TssSignigMsg, poolAddress sdk.AccAddress) (bool, string, error) {
	gasWanted := oc.GetGasEstimation()
	txBuilder, err := oc.genSendTx(operator, []sdk.Msg{sendMsg}, accSeq, accNum, uint64(gasWanted), signMsg)
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to generate the tx")
		return false, "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	txBytes, err := oc.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to encode the tx")
		return false, "", err
	}

	err = nil
	if signMsg != nil {
		err = oc.waitAndSend(poolAddress, accSeq)
	}
	if err == nil {
		isTssMsg := true
		if signMsg == nil {
			isTssMsg = false
		}
		ok, resp, err := oc.BroadcastTx(ctx, txBytes, isTssMsg)
		return ok, resp, err
	}
	return false, "", err
}
