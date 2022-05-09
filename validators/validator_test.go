package validators

import (
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	types2 "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto/ed25519"
	cryptoenc "github.com/tendermint/tendermint/crypto/encoding"
	"testing"
)

func TestNewValidator(t *testing.T) {
	validatorSet := NewValidator()

	sk1 := ed25519.GenPrivKey()
	addr1, err := types.ConsAddressFromHex(sk1.PubKey().Address().String())
	require.Nil(t, err)

	sk2 := ed25519.GenPrivKey()
	addr2, err := types.ConsAddressFromHex(sk2.PubKey().Address().String())
	require.Nil(t, err)

	sk3 := ed25519.GenPrivKey()
	addr3, err := types.ConsAddressFromHex(sk3.PubKey().Address().String())
	require.Nil(t, err)

	sk4 := ed25519.GenPrivKey()
	addr4, err := types.ConsAddressFromHex(sk4.PubKey().Address().String())
	require.Nil(t, err)

	v1 := Validator{
		addr1,
		[]byte("testpubkey1"),
		11,
	}

	v2 := Validator{
		addr2,
		[]byte("testpubkey2"),
		12,
	}

	v3 := Validator{
		addr3,
		[]byte("testpubkey3"),
		13,
	}

	validatorSet.SetupValidatorSet([]*Validator{&v1, &v2, &v3}, 10)
	v, ok := validatorSet.activeValidators[addr1.String()]
	require.True(t, ok)
	require.Equal(t, v.Address, addr1)

	// now we test update validators
	pk4, err := cryptoenc.PubKeyToProto(sk4.PubKey())
	assert.NoError(t, err)

	uv1 := types2.ValidatorUpdate{
		PubKey: pk4,
		Power:  10,
	}

	pk2, err := cryptoenc.PubKeyToProto(sk2.PubKey())
	assert.NoError(t, err)

	uv2 := types2.ValidatorUpdate{
		PubKey: pk2,
	}

	err = validatorSet.UpdateValidatorSet([]types2.ValidatorUpdate{uv1, uv2}, 20)
	require.Nil(t, err)

	validators, height := validatorSet.GetActiveValidators()
	for k := range validators {
		require.NotEqual(t, k, addr2)
	}
	require.Equal(t, height, int64(20))

	// the new validator is in the set
	_, ok = validatorSet.activeValidators[addr4.String()]
	require.True(t, ok)

}
