package pubchain

import (
	"errors"
	"fmt"

	"gitlab.com/joltify/joltifychain-bridge/config"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Verify is the function  to verify the correctness of the account on joltify_bridge
func (a *inboundTx) Verify() error {
	if a.fee.Denom != config.InBoundDenomFee {
		return fmt.Errorf("invalid inbound fee denom with fee demo : %v and want %v", a.fee.Denom, config.InBoundDenom)
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
