package pubchain

import (
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Verify is the function  to verify the correctness of the account on joltify_bridge
func (a *bridgeTx) Verify() error {
	if a.direction == inBound {
		if a.fee.Denom != inBoundDenom {
			return errors.New("invalid fee denom")
		}
		amount, err := sdk.NewDecFromStr(inBoundFeeMin)
		if err != nil {
			return errors.New("invalid minimal inbound fee")
		}
		if a.fee.Amount.LT(sdk.NewIntFromBigInt(amount.BigInt())) {
			return errors.New("the fee is not enough")
		}
	}
	return nil
}
