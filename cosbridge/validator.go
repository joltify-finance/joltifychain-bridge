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
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/validators"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/rest"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	zlog "github.com/rs/zerolog/log"
)

// InitValidators initialize the validators
func (oc *OppyChainInstance) InitValidators(addr string) error {
	ts := tmservice.NewServiceClient(oc.GrpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	for {
		result, err := ts.GetSyncing(ctx, &tmservice.GetSyncingRequest{})
		if err != nil {
			oc.logger.Error().Err(err).Msgf("fail to sync the blocks")
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
	oc.logger.Info().Msgf(">>>>>>>>>>>>>>>>node %v attached>>>>>>>> network %v \n", nodeInfo.GetDefaultNodeInfo().Moniker, nodeInfo.DefaultNodeInfo.Network)

	restRes, err := rest.GetRequest(fmt.Sprintf("%s/status", addr))
	if err != nil {
		return err
	}
	var info info
	err = json.Unmarshal(restRes, &info)
	if err != nil {
		return err
	}

	blockHeight, values, err := QueryTipValidator(oc.GrpcClient)
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to initialize the validator pool")
		return err
	}
	return oc.doInitValidator(info, blockHeight, values)
}

// UpdateLatestValidator update the validator set
func (oc *OppyChainInstance) UpdateLatestValidator(validators []*vaulttypes.Validator, blockHeight int64) error {
	return oc.validatorSet.UpdateValidatorSet(validators, blockHeight)
}

// GetLastValidator get the last validator set
func (oc *OppyChainInstance) GetLastValidator() ([]*validators.Validator, int64) {
	allValidators, blockHeight := oc.validatorSet.GetActiveValidators()
	return allValidators, blockHeight
}

// QueryLastPoolAddress returns the latest two pool outReceiverAddress
func (oc *OppyChainInstance) QueryLastPoolAddress(conn grpc1.ClientConn) ([]*vaulttypes.PoolInfo, error) {
	poolInfo, err := queryLastValidatorSet(conn)
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to get the pool info")
		return nil, err
	}
	return poolInfo, nil
}

// CheckWhetherSigner check whether the current signer is the
func (oc *OppyChainInstance) CheckWhetherSigner(lastPoolInfo *vaulttypes.PoolInfo) (bool, error) {
	found := false
	creator, err := oc.Keyring.Key("operator")
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to get the validator outReceiverAddress")
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
func (oc *OppyChainInstance) CheckWhetherAlreadyExist(conn grpc1.ClientConn, index string) bool {
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
func (oc *OppyChainInstance) CheckTxStatus(conn grpc1.ClientConn, index string, retryNum uint64) error {
	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(submitBackoff), retryNum)

	op := func() error {
		if oc.CheckWhetherAlreadyExist(conn, index) {
			return nil
		}
		return errors.New("fail to find the tx")
	}

	err := backoff.Retry(op, bf)
	return err
}

func (oc *OppyChainInstance) getValidators(height string) ([]*vaulttypes.Validator, error) {
	vaultQuery := vaulttypes.NewQueryClient(oc.GrpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	q := vaulttypes.QueryGetValidatorsRequest{Height: height}
	vaultResp, err := vaultQuery.GetValidators(ctx, &q)
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to query the validators")
		return nil, err
	}
	return vaultResp.Validators.AllValidators, nil
}

func (oc *OppyChainInstance) doInitValidator(i info, blockHeight int64, values []*tmservice.Validator) error {
	oc.myValidatorInfo = i
	oc.validatorSet = validators.NewValidator()

	encCfg := MakeEncodingConfig()
	localVals := make([]*validators.Validator, len(values))
	for index, el := range values {
		var pk cryptotypes.PubKey
		if err := encCfg.InterfaceRegistry.UnpackAny(el.PubKey, &pk); err != nil {
			return err
		}
		adr, err := types.ConsAddressFromBech32(el.Address)
		if err != nil {
			oc.logger.Error().Err(err).Msg("fail to decode the outReceiverAddress")
			return err
		}
		e := validators.Validator{
			Address:     adr,
			PubKey:      pk.Bytes(),
			VotingPower: el.VotingPower,
		}
		localVals[index] = &e
	}

	oc.validatorSet.SetupValidatorSet(localVals, blockHeight)
	return nil
}
