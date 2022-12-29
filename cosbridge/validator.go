package cosbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	grpc1 "github.com/gogo/protobuf/grpc"
	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/validators"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/rest"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	zlog "github.com/rs/zerolog/log"
)

// InitValidators initialize the validators
func (jc *JoltChainInstance) InitValidators(addr string) error {
	ts := tmservice.NewServiceClient(jc.GrpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	for {
		result, err := ts.GetSyncing(ctx, &tmservice.GetSyncingRequest{})
		if err != nil {
			jc.logger.Error().Err(err).Msgf("fail to sync the blocks")
			return err
		}
		if !result.GetSyncing() {
			break
		}
		zlog.Logger.Info().Msg("the blockchain is not fully synced, please wait....")
		time.Sleep(time.Second * 10)
	}

	nodeInfo, err := ts.GetNodeInfo(ctx, &tmservice.GetNodeInfoRequest{})
	if err != nil {
		return err
	}
	config.ChainID = nodeInfo.DefaultNodeInfo.Network
	jc.logger.Info().Msgf(">>>>>>>>>>>>>>>>node %v attached>>>>>>>> network %v \n", nodeInfo.GetDefaultNodeInfo().Moniker, nodeInfo.DefaultNodeInfo.Network)

	restRes, err := rest.GetRequest(fmt.Sprintf("%s/status", addr))
	if err != nil {
		return err
	}
	var info info
	err = json.Unmarshal(restRes, &info)
	if err != nil {
		return err
	}

	blockHeight, values, err := QueryTipValidator(jc.GrpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to initialize the validator pool")
		return err
	}
	return jc.doInitValidator(info, blockHeight, values)
}

// UpdateLatestValidator update the validator set
func (jc *JoltChainInstance) UpdateLatestValidator(validators []*vaulttypes.Validator, blockHeight int64) error {
	return jc.validatorSet.UpdateValidatorSet(validators, blockHeight)
}

// GetLastValidator get the last validator set
func (jc *JoltChainInstance) GetLastValidator() ([]*validators.Validator, int64) {
	allValidators, blockHeight := jc.validatorSet.GetActiveValidators()
	return allValidators, blockHeight
}

// QueryLastPoolAddress returns the latest two pool outReceiverAddress
func (jc *JoltChainInstance) QueryLastPoolAddress(conn grpc1.ClientConn) ([]*vaulttypes.PoolInfo, error) {
	poolInfo, err := queryLastValidatorSet(conn)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to get the pool info")
		return nil, err
	}
	return poolInfo, nil
}

// CheckWhetherSigner check whether the current signer is the
func (jc *JoltChainInstance) CheckWhetherSigner(lastPoolInfo *vaulttypes.PoolInfo) (bool, error) {
	found := false
	creator, err := jc.Keyring.Key("operator")
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to get the validator outReceiverAddress")
		return false, err
	}
	for _, eachValidator := range lastPoolInfo.CreatePool.Nodes {
		if eachValidator.Equals(creator.GetAddress()) {
			found = true
			break
		}
	}

	return found, nil
}

// CheckWhetherAlreadyExist check whether it is already existed
func (jc *JoltChainInstance) CheckWhetherAlreadyExist(conn grpc1.ClientConn, index string) bool {
	ret, err := queryGivenToeknIssueTx(conn, index)
	if err != nil {
		return false
	}
	if ret != nil {
		return true
	}
	return false
}

// CheckTxStatus check whether the tx has been done successfully
func (jc *JoltChainInstance) CheckTxStatus(conn grpc1.ClientConn, index string, retryNum uint64) error {
	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(submitBackoff), retryNum)

	op := func() error {
		if jc.CheckWhetherAlreadyExist(conn, index) {
			return nil
		}
		return errors.New("fail to find the tx")
	}

	err := backoff.Retry(op, bf)
	return err
}

func (jc *JoltChainInstance) getValidators(height string) ([]*vaulttypes.Validator, error) {
	vaultQuery := vaulttypes.NewQueryClient(jc.GrpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	q := vaulttypes.QueryGetValidatorsRequest{Height: height}
	vaultResp, err := vaultQuery.GetValidators(ctx, &q)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to query the validators")
		return nil, err
	}
	return vaultResp.Validators.AllValidators, nil
}

func (jc *JoltChainInstance) doInitValidator(i info, blockHeight int64, values []*tmservice.Validator) error {
	jc.myValidatorInfo = i
	jc.validatorSet = validators.NewValidator()

	encCfg := MakeEncodingConfig()
	localVals := make([]*validators.Validator, len(values))
	for index, el := range values {
		var pk cryptotypes.PubKey
		if err := encCfg.InterfaceRegistry.UnpackAny(el.PubKey, &pk); err != nil {
			return err
		}
		adr, err := types.ConsAddressFromBech32(el.Address)
		if err != nil {
			jc.logger.Error().Err(err).Msg("fail to decode the outReceiverAddress")
			return err
		}
		e := validators.Validator{
			Address:     adr,
			PubKey:      pk.Bytes(),
			VotingPower: el.VotingPower,
		}
		localVals[index] = &e
	}

	jc.validatorSet.SetupValidatorSet(localVals, blockHeight)
	return nil
}
