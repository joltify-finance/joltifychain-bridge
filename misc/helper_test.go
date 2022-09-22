package misc

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/oppyfinance/tss/keysign"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" //nolint
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

	// we generate the eth address from oppy
	poolPk := legacybech32.MustMarshalPubKey(legacybech32.AccPK, &cpk) //nolint
	ethAddr, err := PoolPubKeyToEthAddress(poolPk)
	assert.NoError(t, err)

	addrOppy, err := types.AccAddressFromHex(cpk.Address().String())

	require.Nil(t, err)
	require.EqualValues(t, ethAddr.Hex(), expectedAddr.Hex())
	require.EqualValues(t, addrOppy.String(), "oppy1txtsnx4gr4effr8542778fsxc20j5vzq7wu7r7")
}

func TestGetOppyAddressFromETHSignature(t *testing.T) {
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

	OppyAddr, err := types.AccAddressFromHex(cpk.Address().String())
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

	expectedOppyAddr, err := types.AccAddressFromHex(pk3.Address().String())
	require.Nil(t, err)
	require.True(t, expectedOppyAddr.Equals(OppyAddr))
}

func TestMakeSignature(t *testing.T) {
	SetupBech32Prefix()
	client, err := ethclient.Dial(WebsocketTest)
	assert.Nil(t, err)
	h := common.HexToHash("0x1ec2e9021b0e6d288d61d8d7447493409017174c63c33b95bf9882785fefd944")
	tx, _, err := client.TransactionByHash(context.Background(), h)
	assert.Nil(t, err)

	v, r, s := tx.RawSignatureValues()
	signer := ethTypes.LatestSignerForChainID(tx.ChainId())
	plainV := RecoverRecID(tx.ChainId().Uint64(), v)
	sigBytes := MakeSignature(r, s, plainV)

	sigPublicKey, err := crypto.Ecrecover(signer.Hash(tx).Bytes(), sigBytes)
	assert.Nil(t, err)

	transferFrom, err := EthSignPubKeyToOppyAddr(sigPublicKey)
	assert.Nil(t, err)
	assert.Equal(t, "oppy1q039ggfhyfmx4nrxsl256p2806g8vmg003ht9y", transferFrom.String())

	// now we test serialize the signature

	rEncode := base64.StdEncoding.EncodeToString(r.Bytes())
	sEncode := base64.StdEncoding.EncodeToString(s.Bytes())
	vEncode := base64.StdEncoding.EncodeToString(plainV.Bytes())

	sig := keysign.Signature{
		Msg:        "non_sense_msg",
		R:          rEncode,
		S:          sEncode,
		RecoveryID: vEncode,
	}

	out, err := SerializeSig(&sig, false)
	assert.NoError(t, err)

	assert.True(t, bytes.Equal(out, sigBytes[:len(out)]))

	out, err = SerializeSig(&sig, true)
	assert.NoError(t, err)
	assert.True(t, bytes.Equal(out, sigBytes))
}

func TestPubkeyToOppyAddress(t *testing.T) {
	poolkey := "oppypub1addwnpepqgqd9v2x3axlkkmv5zj8hr6z7pl4fyt8xmhdmfx5kzql2uu6xfdwcahd8ah"
	addr, err := PoolPubKeyToOppyAddress(poolkey)
	assert.NoError(t, err)
	assert.Equal(t, addr.String(), "oppy106q2q2k37jum8zua08yp7lr6llygl3m8933692")

	poolkeyWrong := "oppypub1addwnpepqgqd9v2x3axlkkmv5zj8hr6z7pl4fyt8xmhdxfx5kzql2uu6xfdwcahd8ah"
	_, err = PoolPubKeyToOppyAddress(poolkeyWrong)
	assert.Error(t, err)
}
