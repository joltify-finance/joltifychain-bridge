package misc

import (
	"crypto/ecdsa"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/btcd/btcec"
)

func TestPoolPubkeyToEthAddress(t *testing.T) {
	SetupBech32Prefix()
	privkey991 := "64285613d3569bcaa7a24c9d74d4cec5c18dcf6a08e4c0f66596078f3a4a35b5"
	privateKey, err := crypto.HexToECDSA(privkey991)
	if err != nil {
		panic(err)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		panic("fail to convert to ecdsa pubkey")
	}

	expectedAddr := crypto.PubkeyToAddress(*publicKeyECDSA)

	pkCompressed := crypto.CompressPubkey(publicKeyECDSA)
	cpk := secp256k1.PubKey{
		Key: pkCompressed,
	}

	// we generate the eth address from jolt
	poolPk := legacybech32.MustMarshalPubKey(legacybech32.AccPK, &cpk)
	ethAddr, err := PoolPubKeyToEthAddress(poolPk)

	addrJolt, err := types.AccAddressFromHex(cpk.Address().String())

	require.Nil(t, err)
	require.EqualValues(t, ethAddr.Hex(), expectedAddr.Hex())
	require.EqualValues(t, addrJolt.String(), "jolt1txtsnx4gr4effr8542778fsxc20j5vzqxet7t0")
}

func TestGetJoltAddressFromETHSignature(t *testing.T) {
	ske := "64285613d3569bcaa7a24c9d74d4cec5c18dcf6a08e4c0f66596078f3a4a35b5"
	// skECDSA, err := crypto.HexToECDSA(hex.EncodeToString(sk.Bytes()))
	skECDSA, err := crypto.HexToECDSA(ske)
	require.Nil(t, err)

	publicKey := skECDSA.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		panic("err")
	}
	pkCompressed := crypto.CompressPubkey(publicKeyECDSA)
	cpk := secp256k1.PubKey{
		Key: pkCompressed,
	}

	JoltAddr, err := types.AccAddressFromHex(cpk.Address().String())
	require.Nil(t, err)

	data := []byte("hello")
	hash := crypto.Keccak256Hash(data)

	signature, err := crypto.Sign(hash.Bytes(), skECDSA)
	require.Nil(t, err)

	sigPublicKey, err := crypto.Ecrecover(hash.Bytes(), signature)
	require.Nil(t, err)

	pubkeystrc, err := crypto.UnmarshalPubkey(sigPublicKey)
	assert.Nil(t, err)

	address := crypto.PubkeyToAddress(*pubkeystrc)
	assert.Equal(t, address.Hex(), "0xbDf7Fb0Ad9b0D722ea54D808b79751608E7AE991")
	pk2, err := btcec.ParsePubKey(sigPublicKey, btcec.S256())
	require.Nil(t, err)

	pk3 := secp256k1.PubKey{Key: pk2.SerializeCompressed()}

	expectedJoltAddr, err := types.AccAddressFromHex(pk3.Address().String())
	require.Nil(t, err)
	require.True(t, expectedJoltAddr.Equals(JoltAddr))
}
