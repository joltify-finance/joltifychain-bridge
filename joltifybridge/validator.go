package joltifybridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gitlab.com/joltify/joltifychain-bridge/validators"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/rest"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	zlog "github.com/rs/zerolog/log"

	tmtypes "github.com/tendermint/tendermint/types"
)

// InitValidators initialize the validators
func (jc *JoltifyChainBridge) InitValidators(addr string) error {
	ts := tmservice.NewServiceClient(jc.grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	for {
		result, err := ts.GetSyncing(ctx, &tmservice.GetSyncingRequest{})
		if err != nil {
			return err
		}
		if !result.GetSyncing() {
			break
		}
		zlog.Logger.Info().Msg("the blockchain is not fully synced, please wait....")
		time.Sleep(time.Second * 5)
	}

	nodeInfo, err := ts.GetNodeInfo(ctx, &tmservice.GetNodeInfoRequest{})
	if err != nil {
		return err
	}
	jc.logger.Info().Msgf(">>>>>>>>>>>>>>>>node %v attached>>>>>>>>\n", nodeInfo.GetDefaultNodeInfo().Moniker)

	restRes, err := rest.GetRequest(fmt.Sprintf("%s/status", addr))
	if err != nil {
		return err
	}
	var info info
	err = json.Unmarshal(restRes, &info)
	if err != nil {
		return err
	}

	blockHeight, values, err := QueryTipValidator(jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to initialize the validator pool")
		return err
	}
	return jc.doInitValidator(info, blockHeight, values)
}

// UpdateLatestValidator update the validator set
func (jc *JoltifyChainBridge) UpdateLatestValidator(validators []*tmtypes.Validator, blockHeight int64) error {
	return jc.validatorSet.UpdateValidatorSet(validators, blockHeight)
}

// GetLastValidator get the last validator set
func (jc *JoltifyChainBridge) GetLastValidator() ([]*validators.Validator, int64) {
	validators, blockHeight := jc.validatorSet.GetActiveValidators()
	return validators, blockHeight
}

// QueryLastPoolAddress returns the latest two pool outReceiverAddress
func (jc *JoltifyChainBridge) QueryLastPoolAddress() ([]*vaulttypes.PoolInfo, error) {
	poolInfo, err := queryLastValidatorSet(jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to get the pool info")
		return nil, err
	}
	return poolInfo, nil
}

// CheckWhetherSigner check whether the current signer is the
func (jc *JoltifyChainBridge) CheckWhetherSigner() (bool, error) {
	found := false
	poolInfo, err := jc.QueryLastPoolAddress()
	if err != nil || len(poolInfo) == 0 {
		return found, errors.New("fail to query the signer")
	}
	lastPoolInfo := poolInfo[0]
	creator, err := jc.Keyring.Key("operator")
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to get the validator outReceiverAddress")
		return found, err
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
func (jc *JoltifyChainBridge) CheckWhetherAlreadyExist(index string) bool {
	ret, err := queryGivenToeknIssueTx(jc.grpcClient, index)
	if err != nil {
		jc.logger.Warn().Err(err).Msg("fail to query token with given index")
		return false
	}
	if ret != nil {
		return true
	}
	return false
}

func (jc *JoltifyChainBridge) doInitValidator(i info, blockHeight int64, values []*tmservice.Validator) error {
	jc.myValidatorInfo = i
	jc.validatorSet = validators.NewValidator()

	encCfg := MakeEncodingConfig()
	var localVals []*validators.Validator
	for _, el := range values {
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
		localVals = append(localVals, &e)

	}

	jc.validatorSet.SetupValidatorSet(localVals, blockHeight)
	return nil
}
