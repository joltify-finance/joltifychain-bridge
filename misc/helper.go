package misc

import (
	"encoding/base64"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"

	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/joltgeorge/tss/keysign"
	"github.com/tendermint/btcd/btcec"
)

// SetupBech32Prefix sets up the prefix of the joltify chain
func SetupBech32Prefix() {
	config := types.GetConfig()
	config.SetBech32PrefixForAccount("jolt", "joltpub")
	config.SetBech32PrefixForValidator("joltval", "joltvpub")
	config.SetBech32PrefixForConsensusNode("joltvalcons", "joltcpub")
}

//PoolPubKeyToJoltAddress return the jolt encoded pubkey
func PoolPubKeyToJoltAddress(pk string) (types.AccAddress, error) {
	pubkey, err := types.GetPubKeyFromBech32(types.Bech32PubKeyTypeAccPub, pk)
	if err != nil {
		return types.AccAddress{}, err
	}
	addr, err := types.AccAddressFromHex(pubkey.Address().String())
	return addr, err
}

// PoolPubKeyToEthAddress export the joltify pubkey to the ETH format address
func PoolPubKeyToEthAddress(pk string) (common.Address, error) {
	pubkey, err := types.GetPubKeyFromBech32(types.Bech32PubKeyTypeAccPub, pk)
	if err != nil {
		return common.Address{}, err
	}

	pk2, err := btcec.ParsePubKey(pubkey.Bytes(), btcec.S256())
	if err != nil {
		return common.Address{}, err
	}

	addr := crypto.PubkeyToAddress(*pk2.ToECDSA())

	return addr, nil
}

// EthAddressToJoltAddr export the joltify pubkey to the ETH format address
func EthAddressToJoltAddr(addr common.Address) (types.AccAddress, error) {
	jAddr, err := types.AccAddressFromHex(addr.Hex()[2:])
	return jAddr, err
}

//SerializeSig for both joltify chain and public chain
func SerializeSig(sig *keysign.Signature, needRecovery bool) ([]byte, error) {
	rBytes, err := base64.StdEncoding.DecodeString(sig.R)
	if err != nil {
		return nil, err
	}
	sBytes, err := base64.StdEncoding.DecodeString(sig.S)
	if err != nil {
		return nil, err
	}

	vBytes, err := base64.StdEncoding.DecodeString(sig.RecoveryID)
	if err != nil {
		return nil, err
	}

	R := new(big.Int).SetBytes(rBytes)
	S := new(big.Int).SetBytes(sBytes)
	V := new(big.Int).SetBytes(vBytes)
	N := btcec.S256().N
	halfOrder := new(big.Int).Rsh(N, 1)
	// see: https://github.com/ethereum/go-ethereum/blob/f9401ae011ddf7f8d2d95020b7446c17f8d98dc1/crypto/signature_nocgo.go#L90-L93
	if S.Cmp(halfOrder) == 1 {
		S.Sub(N, S)
	}

	// Serialize signature to R || S.
	// R, S are padded to 32 bytes respectively.
	rBytes = R.Bytes()
	sBytes = S.Bytes()
	vBytes = V.Bytes()

	if needRecovery {
		sigBytes := make([]byte, 65)
		// 0 pad the byte arrays from the left if they aren't big enough.
		copy(sigBytes[32-len(rBytes):32], rBytes)
		copy(sigBytes[64-len(sBytes):64], sBytes)
		copy(sigBytes[65-len(vBytes):65], vBytes)
		return sigBytes, nil
	}
	sigBytes := make([]byte, 64)
	// 0 pad the byte arrays from the left if they aren't big enough.
	copy(sigBytes[32-len(rBytes):32], rBytes)
	copy(sigBytes[64-len(sBytes):64], sBytes)
	return sigBytes, nil
}
