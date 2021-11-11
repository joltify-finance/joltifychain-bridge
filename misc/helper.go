package misc

import (
	"encoding/base64"
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

// PoolPubKeyToEthAddress export the joltify pubkey to the ETH format address
func PoolPubKeyToEthAddress(pk string) (common.Address, error) {
	pubkey, err := types.GetPubKeyFromBech32(types.Bech32PubKeyTypeAccPub, pk)
	if err != nil {
		return common.Address{}, err
	}
	addr := common.BytesToAddress(pubkey.Address().Bytes())
	return addr, nil
}

// EthAddressToJoltAddr export the joltify pubkey to the ETH format address
func EthAddressToJoltAddr(addr common.Address) (types.AccAddress, error) {
	jAddr, err := types.AccAddressFromHex(addr.Hex()[2:])
	return jAddr, err
}

// SerializeSig signature to R || S.
// R, S are padded to 32 bytes respectively.
//func SerializeSig(sig *keysign.Signature) ([]byte, error) {
//	rBytes, errR := base64.StdEncoding.DecodeString(sig.R)
//	sBytes, errS := base64.StdEncoding.DecodeString(sig.S)
//
//	if errR != nil || errS != nil {
//		return nil, errors.New("fail to decode r or s")
//	}
//
//	sigBytes := make([]byte, 64)
//	// 0 pad the byte arrays from the left if they aren't big enough.
//	copy(sigBytes[32-len(rBytes):32], rBytes)
//	copy(sigBytes[64-len(sBytes):64], sBytes)
//	return sigBytes, nil
//}

func SerializeSig(sig *keysign.Signature) ([]byte, error) {
	rBytes, err := base64.StdEncoding.DecodeString(sig.R)
	if err != nil {
		return nil, err
	}
	sBytes, err := base64.StdEncoding.DecodeString(sig.S)
	if err != nil {
		return nil, err
	}

	R := new(big.Int).SetBytes(rBytes)
	S := new(big.Int).SetBytes(sBytes)
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

	sigBytes := make([]byte, 64)
	// 0 pad the byte arrays from the left if they aren't big enough.
	copy(sigBytes[32-len(rBytes):32], rBytes)
	copy(sigBytes[64-len(sBytes):64], sBytes)
	return sigBytes, nil
}
