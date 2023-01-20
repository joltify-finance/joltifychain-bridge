package common

import (
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

func TestAddressConvert(t *testing.T) {
	misc.SetupBech32Prefix()
	a := "cosmos1txtsnx4gr4effr8542778fsxc20j5vzqvekzcx"
	addr, err := AddressStringToBytes("cosmos", a)
	assert.NoError(t, err)
	b := sdk.MustBech32ifyAddressBytes("jolt", addr.Bytes())
	assert.Equal(t, "jolt1txtsnx4gr4effr8542778fsxc20j5vzqxet7t0", b)
}

func TestAddressC2(t *testing.T) {
	misc.SetupBech32Prefix()
	a := "jolt1xdpg5l3pxpyhxqg4ey4krq2pf9d3sphmmuuugg"
	b, err := sdk.AccAddressFromBech32(a)
	assert.NoError(t, err)
	c := sdk.MustBech32ifyAddressBytes("cosmos", b)
	fmt.Printf(">>%v\n", c)
}
