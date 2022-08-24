package common

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
)

func TestAdjustment(t *testing.T) {
	a := sdk.NewInt(101)
	result := AdjustInt(a, 12)
	assert.Equal(t, result.String(), "101000000000000")

	result = AdjustInt(a, 0)
	assert.Equal(t, result.String(), "101")

	result = AdjustInt(a, -1)
	assert.Equal(t, result.String(), "10")

	result = AdjustInt(a, -3)
	assert.Equal(t, result.String(), "0")

	result = AdjustInt(a, -4)
	assert.Equal(t, result.String(), "0")

	// 1/3*3 should be 0
	b := AdjustInt(sdk.NewInt(1), 12)
	c := b.Quo(sdk.NewInt(3))
	d := c.Mul(sdk.NewInt(3))
	final := AdjustInt(d, -12)
	assert.Equal(t, final.String(), "0")
}
