package pubchain

import (
	"errors"

	"gitlab.com/joltify/joltifychain-bridge/config"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Verify is the function  to verify the correctness of the account on joltify_bridge
func (a *inboundTx) Verify() error {
	if a.fee.Denom != config.InBoundDenom {
		return errors.New("invalid inbound fee denom")
	}
	amount, err := sdk.NewDecFromStr(config.InBoundFeeMin)
	if err != nil {
		return errors.New("invalid minimal inbound fee")
	}
	if a.fee.Amount.LT(sdk.NewIntFromBigInt(amount.BigInt())) {
		return errors.New("the fee is not enough")
	}
	return nil
}
