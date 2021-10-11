package bridge

import (
	"context"
	"invoicebridge/tssclient"

	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"

	"github.com/cosmos/cosmos-sdk/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtypes "github.com/tendermint/tendermint/types"
)

const capacity = 10000

func (ic *InvChainBridge) AddSubscribe(ctx context.Context, query string) (<-chan ctypes.ResultEvent, error) {
	out, err := ic.wsClient.Subscribe(ctx, "test", query, capacity)
	if err != nil {
		ic.logger.Error().Msgf("Failed to subscribe to query", "err", err, "query", query)
		return nil, err
	}

	return out, nil
}

func (ic *InvChainBridge) HandleUpdateValidators(validatorUpdates []*tmtypes.Validator, height int64) error {
	err := ic.UpdateLatestValidator(validatorUpdates, height)
	if err != nil {
		ic.logger.Error().Msgf("fail to query the latest validator %v", err)
		return err
	}

	lastValidators, blockHeight := ic.GetLastValidator()

	ic.logger.Info().Msgf(">>>>>>>>>>>>>>>>at block height %v system do keygen>>>>>>>>>>>>>>>\n", blockHeight)

	// fixme we need to check the key type
	privkey, err := ImportPrivKey(ic.cosKey.PrivKey.Value)
	if err != nil {
		ic.logger.Error().Err(err).Msg("fail to import the cosmos private key")
		return err
	}

	myPk, _ := types.Bech32ifyPubKey(sdk.Bech32PubKeyTypeConsPub, privkey.PubKey())

	var pubkeys []string
	doKeyGen := false
	for _, el := range lastValidators {

		key := ed25519.PubKey{
			Key: el.PubKey,
		}

		var ret cryptotypes.PubKey
		ret = &key

		pk, _ := types.Bech32ifyPubKey(sdk.Bech32PubKeyTypeConsPub, ret)
		if pk == myPk {
			doKeyGen = true
		}
		pubkeys = append(pubkeys, pk)
	}
	if doKeyGen {
		resp, err := ic.tssServer.Keygen(pubkeys, blockHeight, tssclient.Version)
		if err != nil {
			ic.logger.Error().Err(err).Msg("fail to do the keygen")
			return err
		}
		ic.logger.Info().Msgf(">>>>%v\n", resp.PoolAddress)
	}
	return nil
}
