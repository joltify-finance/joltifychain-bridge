package misc

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestPoolPubkeyToEthAddress(t *testing.T) {
	SetupBech32Prefix()
	sk := secp256k1.GenPrivKey()
	addr := sk.PubKey().Address()
	joltAddr, err := types.AccAddressFromHex(addr.String())
	require.Nil(t, err)

	pk, err := types.Bech32ifyPubKey(types.Bech32PubKeyTypeAccPub, sk.PubKey())
	require.Nil(t, err)

	ethAddr, err := PoolPubKeyToEthAddress(pk)
	require.Nil(t, err)
	require.EqualValues(t, ethAddr.Bytes(), joltAddr.Bytes())

	jAddr, err := EthAddressToJoltAddr(ethAddr)
	require.Nil(t, err)
	require.EqualValues(t, jAddr, joltAddr)
}
