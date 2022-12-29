package common

import (
	"math/big"

	"github.com/cosmos/cosmos-sdk/types"
)

func doGetFee(price, ceil, floor, feeRatio types.Dec, transferAmount types.Int, decimal int64) types.Int {

	feeAmount := transferAmount.ToDec().Mul(feeRatio).RoundInt()

	val := new(big.Int).Exp(big.NewInt(10), new(big.Int).Abs(big.NewInt(decimal)), nil)

	ceilAmount := ceil.MulInt(types.NewIntFromBigInt(val)).Quo(price).RoundInt()
	floorAmount := floor.MulInt(types.NewIntFromBigInt(val)).Quo(price).RoundInt()

	if feeAmount.LT(floorAmount) {
		return floorAmount
	}
	if feeAmount.GT(ceilAmount) {
		return ceilAmount
	}
	return feeAmount

}

func CalculateFee(feeModule *FeeModule, price types.Dec, token types.Coin) (types.Coin, error) {
	decimal := int64(18)
	if token.Denom == "ujolt" {
		decimal = int64(6)
	}
	fee := doGetFee(price, feeModule.Ceil, feeModule.Floor, feeModule.FeeRatio, token.Amount, decimal)

	feeCoin := types.NewCoin(token.Denom, fee)
	return feeCoin, nil
}

type FeeModule struct {
	ChainType string
	FeeRatio  types.Dec
	Ceil      types.Dec
	Floor     types.Dec
}

func InitFeeModule() map[string]*FeeModule {
	ethFeeModule := FeeModule{
		ChainType: "ETH",
		FeeRatio:  types.NewDecWithPrec(1, 4),
		Ceil:      types.NewDecWithPrec(1000, 0),
		Floor:     types.NewDecWithPrec(25, 1),
	}

	bscFeeModule := FeeModule{
		ChainType: "BSC",
		FeeRatio:  types.NewDecWithPrec(1, 4),
		Ceil:      types.NewDecWithPrec(1000, 0),
		Floor:     types.NewDecWithPrec(10, 2),
	}
	ret := make(map[string]*FeeModule)
	ret[ethFeeModule.ChainType] = &ethFeeModule
	ret[bscFeeModule.ChainType] = &bscFeeModule
	return ret
}
