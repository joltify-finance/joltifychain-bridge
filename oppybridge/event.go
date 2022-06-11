package oppybridge

import (
	"context"
	"encoding/base64"
	"strconv"

	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	"gitlab.com/oppy-finance/oppy-bridge/tssclient"

	"github.com/oppyfinance/tss/common"

	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"

	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

const capacity = 10000

// AddSubscribe add the subscirbe to the chain
func (jc *OppyChainInstance) AddSubscribe(ctx context.Context, query string) (<-chan ctypes.ResultEvent, error) {
	out, err := jc.wsClient.Subscribe(ctx, "oppyBridge", query, capacity)
	if err != nil {
		jc.logger.Error().Err(err).Msgf("Failed to subscribe to query with error %v", err)
		return nil, err
	}

	return out, nil
}

// HandleUpdateValidators check whether we need to generate the new tss pool message
func (jc *OppyChainInstance) HandleUpdateValidators(height int64) error {
	v, err := jc.getValidators(strconv.FormatInt(height, 10))
	if err != nil {
		return err
	}

	err = jc.UpdateLatestValidator(v, height)
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

		pk, err := legacybech32.MarshalPubKey(legacybech32.AccPK, ret) // nolint
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
		creator, err := jc.Keyring.Key("operator")
		if err != nil {
			jc.logger.Error().Msgf("fail to get the operator key :%v", err)
			return err
		}

		err = jc.prepareTssPool(creator.GetAddress(), resp.PubKey, strconv.FormatInt(blockHeight+1, 10))
		if err != nil {
			jc.logger.Error().Msgf("fail to broadcast the tss generated key on pub_chain")
			return err
		}
		jc.logger.Info().Msgf("successfully prepared the tss key info on pub_chain")
		return nil
	}
	return nil
}
