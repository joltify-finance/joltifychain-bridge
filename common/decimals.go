package common

import (
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func AdjustInt(input sdk.Int, deltaPrecision int64) sdk.Int {
	val := new(big.Int).Exp(big.NewInt(10), new(big.Int).Abs(big.NewInt(deltaPrecision)), nil)
	if deltaPrecision < 0 {
		return input.Quo(sdk.NewIntFromBigInt(val))
	} else {
		return input.Mul(sdk.NewIntFromBigInt(val))
	}
}
