package common

import (
	"math/big"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
)

func TestCalculateFee(t *testing.T) {
	bscFeeModule := FeeModule{
		ChainType: "BSC",
		FeeRatio:  sdk.NewDecWithPrec(1, 4),
		Ceil:      sdk.NewDecWithPrec(1000, 0),
		Floor:     sdk.NewDecWithPrec(10, 2),
	}

	price := sdk.NewDecWithPrec(2557, 1)

	testToken := sdk.NewCoin("bnb", sdk.NewIntWithDecimal(1, 18))
	fee, err := CalculateFee(&bscFeeModule, price, testToken)
	assert.NoError(t, err)

	val := new(big.Int).Exp(big.NewInt(10), new(big.Int).Abs(big.NewInt(18)), nil)
	feePaid := price.Mul(fee.Amount.ToDec()).Quo(sdk.NewDecFromBigInt(val))
	//expected := testToken.Amount.ToDec().Mul(bscFeeModule.FeeRatio).RoundInt()
	gap := feePaid.Sub(bscFeeModule.Floor).Abs()
	assert.True(t, gap.LT(sdk.MustNewDecFromStr("0.000001")))
}

func TestCalculateFeeCeil(t *testing.T) {
	bscFeeModule := FeeModule{
		ChainType: "BSC",
		FeeRatio:  sdk.NewDecWithPrec(1, 4),
		Ceil:      sdk.NewDecWithPrec(1000, 0),
		Floor:     sdk.NewDecWithPrec(10, 2),
	}

	price := sdk.NewDecWithPrec(2557, 1)

	tokenAmount, ok := sdk.NewIntFromString("39108330074306000000000")
	assert.True(t, ok)
	testToken := sdk.NewCoin("bnb", tokenAmount)
	fee, err := CalculateFee(&bscFeeModule, price, testToken)
	assert.NoError(t, err)

	val := new(big.Int).Exp(big.NewInt(10), new(big.Int).Abs(big.NewInt(18)), nil)
	feePaid := price.Mul(fee.Amount.ToDec()).Quo(sdk.NewDecFromBigInt(val))
	//expected := testToken.Amount.ToDec().Mul(bscFeeModule.FeeRatio).RoundInt()
	gap := feePaid.Sub(bscFeeModule.Ceil).Abs()
	assert.True(t, gap.LT(sdk.MustNewDecFromStr("0.000001")))

	// now  a little bit smaller than the ceil
	tokenAmount, ok = sdk.NewIntFromString("39108329064206000000000")
	assert.True(t, ok)
	testToken = sdk.NewCoin("bnb", tokenAmount)
	fee, err = CalculateFee(&bscFeeModule, price, testToken)
	assert.NoError(t, err)

	feePaid = price.Mul(fee.Amount.ToDec()).Quo(sdk.NewDecFromBigInt(val))
	gap = feePaid.Sub(bscFeeModule.Ceil).Abs()
	assert.False(t, gap.LT(sdk.MustNewDecFromStr("0.000001")))

	expected := testToken.Amount.ToDec().Mul(bscFeeModule.FeeRatio).RoundInt()
	assert.True(t, expected.Equal(fee.Amount))
}

func TestCalculateFeeFloor(t *testing.T) {
	bscFeeModule := FeeModule{
		ChainType: "BSC",
		FeeRatio:  sdk.NewDecWithPrec(1, 4),
		Ceil:      sdk.NewDecWithPrec(1000, 0),
		Floor:     sdk.NewDecWithPrec(10, 2),
	}

	price := sdk.NewDecWithPrec(2557, 1)
	tokenAmount, ok := sdk.NewIntFromString("3910833000000000000")
	assert.True(t, ok)
	testToken := sdk.NewCoin("bnb", tokenAmount)
	fee, err := CalculateFee(&bscFeeModule, price, testToken)
	assert.NoError(t, err)

	val := new(big.Int).Exp(big.NewInt(10), new(big.Int).Abs(big.NewInt(18)), nil)
	feePaid := price.Mul(fee.Amount.ToDec()).Quo(sdk.NewDecFromBigInt(val))
	//expected := testToken.Amount.ToDec().Mul(bscFeeModule.FeeRatio).RoundInt()
	gap := feePaid.Sub(bscFeeModule.Floor).Abs()
	assert.True(t, gap.LT(sdk.MustNewDecFromStr("0.000001")))

	// now  a little bit bigger than the floor
	tokenAmount, ok = sdk.NewIntFromString("3910834000000000000")
	assert.True(t, ok)
	testToken = sdk.NewCoin("bnb", tokenAmount)
	fee, err = CalculateFee(&bscFeeModule, price, testToken)
	assert.NoError(t, err)

	feePaid = price.Mul(fee.Amount.ToDec()).Quo(sdk.NewDecFromBigInt(val))
	gap = feePaid.Sub(bscFeeModule.Ceil).Abs()
	assert.False(t, gap.LT(sdk.MustNewDecFromStr("0.000001")))

	expected := testToken.Amount.ToDec().Mul(bscFeeModule.FeeRatio).RoundInt()
	assert.True(t, expected.Equal(fee.Amount))
}

func verify(amount sdk.Int, price sdk.Dec) sdk.Dec {
	val := new(big.Int).Exp(big.NewInt(10), new(big.Int).Abs(big.NewInt(18)), nil)
	realAmount := sdk.NewDecFromInt(amount).Quo(sdk.NewDecFromBigInt(val))
	cost := realAmount.Mul(price)
	return cost
}

func initFeeModule() map[string]*FeeModule {
	ethFeeModule := FeeModule{
		ChainType: "ETH",
		FeeRatio:  sdk.NewDecWithPrec(1, 4),
		Ceil:      sdk.NewDecWithPrec(1000, 0),
		Floor:     sdk.NewDecWithPrec(25, 1),
	}

	bscFeeModule := FeeModule{
		ChainType: "BSC",
		FeeRatio:  sdk.NewDecWithPrec(1, 4),
		Ceil:      sdk.NewDecWithPrec(1000, 0),
		Floor:     sdk.NewDecWithPrec(10, 2),
	}
	ret := make(map[string]*FeeModule)
	ret[ethFeeModule.ChainType] = &ethFeeModule
	ret[bscFeeModule.ChainType] = &bscFeeModule
	return ret
}

func TestDoGetFee(t *testing.T) {
	// mock bnb price
	price := sdk.NewDecWithPrec(24530, 2)

	feeModule := initFeeModule()
	ethFeeModule := feeModule["ETH"]

	val := new(big.Int).Exp(big.NewInt(10), new(big.Int).Abs(big.NewInt(18)), nil)
	transfer := sdk.NewDecWithPrec(10, 2)
	amount := transfer.MulInt(sdk.NewIntFromBigInt(val)).TruncateInt()
	out := doGetFee(price, ethFeeModule.Ceil, ethFeeModule.Floor, ethFeeModule.FeeRatio, amount, int64(18))

	cost := verify(out, price)
	assert.True(t, ethFeeModule.Floor.Sub(cost).Abs().LT(sdk.NewDecWithPrec(1, 4)))

	transfer = sdk.NewDecWithPrec(200, 0)
	amount = transfer.MulInt(sdk.NewIntFromBigInt(val)).TruncateInt()
	out = doGetFee(price, ethFeeModule.Ceil, ethFeeModule.Floor, ethFeeModule.FeeRatio, amount, int64(18))

	cost = verify(out, price)

	expected := transfer.Mul(price).Mul(ethFeeModule.FeeRatio)
	assert.True(t, ethFeeModule.Floor.Sub(expected).LT(sdk.NewDecWithPrec(1, 4)))

	transfer = sdk.NewDecWithPrec(200000, 0)
	amount = transfer.MulInt(sdk.NewIntFromBigInt(val)).TruncateInt()
	out = doGetFee(price, ethFeeModule.Ceil, ethFeeModule.Floor, ethFeeModule.FeeRatio, amount, int64(18))

	cost = verify(out, price)
	assert.True(t, ethFeeModule.Ceil.Sub(cost).LT(sdk.NewDecWithPrec(1, 4)))
}

//
//func TestDoGetFee(t *testing.T) {
//	feeModule := initFeeModule()
//	assert.Equal(t, len(feeModule), 2)
//	expected := sdk.NewDecWithPrec(25, 1)
//	assert.True(t, feeModule["ETH"].Floor.Equal(expected))
//}
