package bridge

import (
	"context"
	"fmt"
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

	ic.logger.Debug().Msgf("\n>>>>>>>>>>>>>>>>block height %v>>>>>>>>>>>>>>>\n", blockHeight)

	addr, err := types.ConsAddressFromHex(ic.cosKey.Address)
	if err != nil {
		ic.logger.Error().Err(err).Msg("fail to decode the validator key")
		return err
	}

	fmt.Printf(">>>>>>>>>>%v\n", ic.cosKey.PrivKey.Type)
	privkey, err := ImportPrivKey(ic.cosKey.PrivKey.Value)
	if err != nil {
		ic.logger.Error().Err(err).Msg("fail to import the cosmos private key")
		return err
	}

	pk, _ := types.Bech32ifyPubKey(sdk.Bech32PubKeyTypeConsPub, privkey.PubKey())
	fmt.Printf(">>>>>>>>>>>>>>>%v\n", pk)

	var pubkeys []string
	for _, el := range lastValidators {
		if addr.Equals(el.Address) {
			fmt.Printf("WWWWWWWWWWWWWW\n")
		}
		key := ed25519.PubKey{
			Key: el.PubKey,
		}

		var ret cryptotypes.PubKey
		ret = &key

		pk, _ := types.Bech32ifyPubKey(sdk.Bech32PubKeyTypeConsPub, ret)
		fmt.Printf("?>?????>>>>%v\n", pk)
		pubkeys = append(pubkeys, pk)
	}
	fmt.Printf("%v\n", pubkeys)
	return nil
}
