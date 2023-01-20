package common

import (
	"errors"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func AddressStringToBytes(prefix, address string) (sdk.AccAddress, error) {
	if len(strings.TrimSpace(address)) == 0 {
		return sdk.AccAddress{}, errors.New("empty address string is not allowed")
	}

	bz, err := sdk.GetFromBech32(address, prefix)
	if err != nil {
		return sdk.AccAddress{}, err
	}

	err = sdk.VerifyAddressFormat(bz)
	if err != nil {
		return sdk.AccAddress{}, err
	}
	return bz, nil
}
