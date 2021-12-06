package joltifybridge

import (
	"context"
	"encoding/base64"
	"strconv"

	"gitlab.com/joltify/joltifychain-bridge/tssclient"

	"github.com/joltgeorge/tss/common"

	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"

	"github.com/cosmos/cosmos-sdk/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtypes "github.com/tendermint/tendermint/types"
)

const capacity = 10000

func (jc *JoltifyChainBridge) AddSubscribe(ctx context.Context, query string) (<-chan ctypes.ResultEvent, error) {
	out, err := jc.wsClient.Subscribe(ctx, "joltifyBridge", query, capacity)
	if err != nil {
		jc.logger.Error().Err(err).Msgf("Failed to subscribe to query with error %v", err)
		return nil, err
	}

	return out, nil
}

// HandleUpdateValidators check whether we need to generate the new tss pool message
func (jc *JoltifyChainBridge) HandleUpdateValidators(validatorUpdates []*tmtypes.Validator, height int64) error {
	err := jc.UpdateLatestValidator(validatorUpdates, height)
	if err != nil {
		jc.logger.Error().Msgf("fail to query the latest validator %v", err)
		return err
	}

	lastValidators, blockHeight := jc.GetLastValidator()

	jc.logger.Info().Msgf(">>>>>>>>>>>>>>>>at block height %v system do keygen>>>>>>>>>>>>>>>\n", blockHeight)

	var pubkeys []string
	doKeyGen := false
	for _, el := range lastValidators {
		key := ed25519.PubKey{
			Key: el.PubKey,
		}
		ret := &key

		pk, err := types.Bech32ifyPubKey(sdk.Bech32PubKeyTypeConsPub, ret)
		if err != nil {
			return err
		}

		pkValidator, err := base64.StdEncoding.DecodeString(jc.myValidatorInfo.Result.ValidatorInfo.PubKey.Value)
		if err != nil {
			return err
		}

		myValidatorPubKey := ed25519.PubKey{
			Key: pkValidator,
		}

		if key.Equals(&myValidatorPubKey) {
			doKeyGen = true
		}
		pubkeys = append(pubkeys, pk)
	}
	if doKeyGen {
		resp, err := jc.tssServer.KeyGen(pubkeys, blockHeight, tssclient.TssVersion)
		if err != nil {
			jc.logger.Error().Err(err).Msg("fail to do the keygen")
			return err
		}
		if resp.Status != common.Success {
			// todo we need to put our blame on pub_chain as well
			jc.logger.Error().Msgf("we fail to ge the valid key")
			return nil
		}
		// now we put the tss key on pub_chain
		creator, err := jc.keyring.Key("operator")
		if err != nil {
			jc.logger.Error().Msgf("fail to get the operator key :%v", err)
			return err
		}

		err = jc.prepareTssPool(creator.GetAddress(), resp.PubKey, strconv.FormatInt(blockHeight+1, 10))
		if err != nil {
			jc.logger.Error().Msgf("fail to broadcast the tss generated key on pub_chain")
			return err
		}
		jc.logger.Info().Msgf("successfully upload the tss key info on pub_chain")
		return nil
	}
	return nil
}
