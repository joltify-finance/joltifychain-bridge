package misc

import (
	"fmt"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/btcd/btcec"
)

func TestPoolPubkeyToEthAddress(t *testing.T) {
	SetupBech32Prefix()
	sk := secp256k1.GenPrivKey()

	pubkey, err := crypto.DecompressPubkey(sk.PubKey().Bytes())
	require.Nil(t, err)

	expectedAddr := crypto.PubkeyToAddress(*pubkey)

	expectedAddr2, err := AccountPubKeyToEthAddress(sk.PubKey())
	require.Nil(t, err)
	pk, err := types.Bech32ifyPubKey(types.Bech32PubKeyTypeAccPub, sk.PubKey())
	require.Nil(t, err)

	ethAddr, err := PoolPubKeyToEthAddress(pk)
	require.Nil(t, err)
	require.EqualValues(t, ethAddr.Bytes(), expectedAddr.Bytes())
	require.EqualValues(t, ethAddr.Bytes(), expectedAddr2.Bytes())
}

func TestGetJoltAddressFromETHSignature(t *testing.T) {
	sk := secp256k1.GenPrivKey()
	ske := "64285613d3569bcaa7a24c9d74d4cec5c18dcf6a08e4c0f66596078f3a4a35b5"
	// skECDSA, err := crypto.HexToECDSA(hex.EncodeToString(sk.Bytes()))
	skECDSA, err := crypto.HexToECDSA(ske)
	require.Nil(t, err)

	JoltAddr, err := types.AccAddressFromHex(sk.PubKey().Address().String())
	require.Nil(t, err)

	data := []byte("hello")
	hash := crypto.Keccak256Hash(data)

	signature, err := crypto.Sign(hash.Bytes(), skECDSA)
	require.Nil(t, err)
	fmt.Printf(">>>%v\n", signature)

	sigPublicKey, err := crypto.Ecrecover(hash.Bytes(), signature)
	require.Nil(t, err)

	pubkeystrc, err := crypto.UnmarshalPubkey(sigPublicKey)
	assert.Nil(t, err)

	address := crypto.PubkeyToAddress(*pubkeystrc)
	fmt.Printf(">>>>>%v\n", address)
	pk2, err := btcec.ParsePubKey(sigPublicKey, btcec.S256())
	require.Nil(t, err)

	pk3 := secp256k1.PubKey{Key: pk2.SerializeCompressed()}

	expectedJoltAddr, err := types.AccAddressFromHex(pk3.Address().String())
	require.Nil(t, err)
	require.True(t, expectedJoltAddr.Equals(JoltAddr))
}
