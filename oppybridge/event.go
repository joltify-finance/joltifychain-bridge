package oppybridge

import (
	"encoding/base64"
	"errors"
	"strconv"

	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	"gitlab.com/oppy-finance/oppy-bridge/tssclient"

	"github.com/oppyfinance/tss/common"

	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
)

const capacity = 10000

// HandleUpdateValidators check whether we need to generate the new tss pool message
func (oc *OppyChainInstance) HandleUpdateValidators(height int64) error {
	v, err := oc.getValidators(strconv.FormatInt(height, 10))
	if err != nil {
		return err
	}

	err = oc.UpdateLatestValidator(v, height)
	if err != nil {
		oc.logger.Error().Msgf("fail to query the latest validator %v", err)
		return err
	}
	lastValidators, blockHeight := oc.GetLastValidator()

	oc.logger.Info().Msgf(">>>>>>>>>>>>>>>>at block height %v system do keygen>>>>>>>>>>>>>>>\n", blockHeight)

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

		pkValidator, err := base64.StdEncoding.DecodeString(oc.myValidatorInfo.Result.ValidatorInfo.PubKey.Value)
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
		resp, err := oc.tssServer.KeyGen(pubkeys, blockHeight, tssclient.TssVersion)
		if err != nil {
			oc.logger.Error().Err(err).Msg("fail to do the keygen")
			return err
		}
		if resp.Status != common.Success {
			// todo we need to put our blame on pub_chain as well
			oc.logger.Error().Msgf("we fail to ge the valid key")
			return errors.New("fail to get the valid key")
		}
		// now we put the tss key on pub_chain
		// fixme we need to allow user to choose the name of the key
		creator, err := oc.Keyring.Key("operator")
		if err != nil {
			oc.logger.Error().Msgf("fail to get the operator key :%v", err)
			return err
		}

		err = oc.prepareTssPool(creator.GetAddress(), resp.PubKey, strconv.FormatInt(blockHeight+1, 10))
		if err != nil {
			oc.logger.Error().Msgf("fail to broadcast the tss generated key on pub_chain")
			return err
		}
		oc.logger.Info().Msgf("successfully prepared the tss key info on pub_chain")
		return nil
	}
	return nil
}
