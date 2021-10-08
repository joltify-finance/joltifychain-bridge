package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/types/rest"
	zlog "github.com/rs/zerolog/log"
	"invoicebridge/validators"
	"time"
)

func (ic *InvChainBridge) InitValidators(addr string) error {

	ts := tmservice.NewServiceClient(ic.grpcClient)
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
	fmt.Printf(">>>>>>>>>>>>>>>>node %v attached>>>>>>>>.\n", nodeInfo.GetDefaultNodeInfo().Moniker)

	restRes, err := rest.GetRequest(fmt.Sprintf("%s/status", addr))
	if err != nil {
		return err
	}
	var info Info
	err = json.Unmarshal(restRes, &info)
	if err != nil {
		return err
	}

	blockHeight, vals, err := QueryHistoricalValidator(ic.grpcClient)
	if err != nil {
		ic.logger.Error().Err(err).Msg("fail to initialize the validator pool")
		return err
	}
	ic.myValidatorInfo = info
	ic.validatorSet = validators.NewValidator()
	ic.validatorSet.SetupValidatorSet(vals, blockHeight)
	return nil
}

func (ic *InvChainBridge) UpdateLatestValidator() error {
	blockHeight, validators, err := QueryHistoricalValidator(ic.grpcClient)
	if err != nil {
		ic.logger.Error().Err(err).Msg("fail to initialize the validator pool")
		return err
	}
	ic.validatorSet.UpdateValidatorSet(validators, blockHeight)
	return nil
}

func (ic *InvChainBridge) GetLatestValidator() ([]*tmservice.Validator, int64) {

	validators, blockHeight := ic.validatorSet.GetActiveValidators()
	return validators, blockHeight
}

//func (ic *InvChainBridge) UpdateValidators(updateValidators []*types.Validator, blockHeight int64) error {
//	var cosUpdateValidators []*tmservice.Validator
//
//	for _, el := range updateValidators {
//
//		types2.NewAnyWithValue(el.PubKey)
//		each := tmservice.Validator{
//			Address:          el.Address.String(),
//			PubKey:           el.PubKey,
//			VotingPower:      el.VotingPower,
//			ProposerPriority: el.ProposerPriority,
//		}
//		cosUpdateValidators = append(cosUpdateValidators, &each)
//	}
//	ic.validatorSet.UpdateValidatorSet(cosUpdateValidators, blockHeight)
//	return nil
//}
