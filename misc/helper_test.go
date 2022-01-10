package misc

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/crypto"
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
	skECDSA, err := crypto.HexToECDSA(hex.EncodeToString(sk.Bytes()))
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

	pk2, err := btcec.ParsePubKey(sigPublicKey, btcec.S256())
	require.Nil(t, err)

	pk3 := secp256k1.PubKey{Key: pk2.SerializeCompressed()}

	expectedJoltAddr, err := types.AccAddressFromHex(pk3.Address().String())
	require.Nil(t, err)
	require.True(t, expectedJoltAddr.Equals(JoltAddr))

	R, _ := new(big.Int).SetString("67606333147281522726157602227936966047617416004627204291913366769272514064697", 10)
	S, _ := new(big.Int).SetString("5475031428760630727513767655728116229184222599154855988385984175155192504097", 10)
	V, _ := new(big.Int).SetString("229", 10)
	r, s := R.Bytes(), S.Bytes()
	sig := make([]byte, crypto.SignatureLength)
	copy(sig[32-len(r):32], r)
	copy(sig[64-len(s):64], s)
	sig[64] = byte(V.Uint64())
	fmt.Printf(">>>>%v:%v\n", sig, len(sig))
}
