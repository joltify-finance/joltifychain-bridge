package joltifybridge

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/pubchain"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/validators"
	types2 "gitlab.com/joltify/joltifychain/joltifychain/x/vault/types"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/rest"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	zlog "github.com/rs/zerolog/log"

	tmtypes "github.com/tendermint/tendermint/types"
)

//InitValidators initialize the validators
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
	jc.doInitValidator(info, blockHeight, values)

	return nil
}

//UpdateLatestValidator update the validator set
func (jc *JoltifyChainBridge) UpdateLatestValidator(validators []*tmtypes.Validator, blockHeight int64) error {
	return jc.validatorSet.UpdateValidatorSet(validators, blockHeight)
}

//GetLastValidator get the last validator set
func (jc *JoltifyChainBridge) GetLastValidator() ([]*validators.Validator, int64) {
	validators, blockHeight := jc.validatorSet.GetActiveValidators()
	return validators, blockHeight
}

//QueryLastPoolAddress returns the latest two pool address
func (jc *JoltifyChainBridge) QueryLastPoolAddress() ([]*types2.PoolInfo, error) {
	poolInfo, err := queryLastValidatorSet(jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to get the pool info")
		return nil, err
	}
	return poolInfo, nil
}

//CheckWhetherSigner check whether the current signer is the
func (jc *JoltifyChainBridge) CheckWhetherSigner(item *pubchain.AccountInboundReq) (bool, error) {
	found := false
	poolInfo, err := jc.QueryLastPoolAddress()
	if err != nil {
		return found, err
	}
	creator, err := jc.keyring.Key("operator")
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to get the validator address")
		return found, err
	}

	_, toPoolAddr, _ := item.GetInboundReqInfo()
	for _, el := range poolInfo {
		pubkey, err := types.GetPubKeyFromBech32(sdk.Bech32PubKeyTypeAccPub, el.GetCreatePool().PoolPubKey)
		if err != nil {
			jc.logger.Error().Err(err).Msg("fail to decode the pool pub key")
			return found, err
		}
		hexPoolPubKey := common.BytesToAddress(pubkey.Address().Bytes())
		if hexPoolPubKey.String() == toPoolAddr.String() {
			for _, eachValidator := range el.CreatePool.Nodes {
				if eachValidator.Equals(creator.GetAddress()) {
					found = true
					break
				}
			}
		}
	}

	return found, nil
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
			jc.logger.Error().Err(err).Msg("fail to decode the address")
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

//func (ic *JoltifyChainBridge) UpdateValidators(updateValidators []*types.Validator, blockHeight int64) error {
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
