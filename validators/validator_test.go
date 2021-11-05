package validators

import (
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
	tmtypes "github.com/tendermint/tendermint/types"

	//tmtypes "github.com/tendermint/tendermint/types"
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

	//now we test update validators

	uv1 := tmtypes.NewValidator(sk4.PubKey(), 10)
	uv2 := tmtypes.NewValidator(sk2.PubKey(), 0)

	err = validatorSet.UpdateValidatorSet([]*tmtypes.Validator{uv1, uv2}, 20)
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
