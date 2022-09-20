package oppybridge

import (
	"crypto/ecdsa"
	"encoding/base64"
	"errors"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/oppyfinance/tss/blame"
	"github.com/oppyfinance/tss/common"
	"github.com/oppyfinance/tss/keygen"
	"github.com/oppyfinance/tss/keysign"
)

type TssMock struct {
	sk             *secp256k1.PrivKey
	keys           keyring.Keyring
	keygenSuccess  bool
	keysignSuccess bool
}

func (tm *TssMock) KeySign(pk string, msgs []string, blockHeight int64, signers []string, version string) (keysign.Response, error) {
	if !tm.keysignSuccess {
		return keysign.NewResponse(nil, common.Fail, blame.NewBlame("", nil)), errors.New("fail in keysign")
	}

	msg, err := base64.StdEncoding.DecodeString(msgs[0])
	if err != nil {
		return keysign.Response{}, err
	}
	var sk *ecdsa.PrivateKey
	if tm.sk != nil {
		sk, err = crypto.ToECDSA(tm.sk.Bytes())
		if err != nil {
			return keysign.Response{}, err
		}
	}
	var signature []byte
	if tm.keys != nil {
		s, _, err := tm.keys.Sign("node0", msg)
		signature = s
		if err != nil {
			return keysign.Response{}, err
		}
	} else {
		signature, err = crypto.Sign(msg, sk)
		if err != nil {
			return keysign.Response{}, err
		}
	}
	r := signature[:32]
	s := signature[32:64]
	var v []byte
	if len(signature) == 64 {
		v = signature[63:]
	} else {
		v = signature[64:65]
	}

	rEncoded := base64.StdEncoding.EncodeToString(r)
	sEncoded := base64.StdEncoding.EncodeToString(s)
	vEncoded := base64.StdEncoding.EncodeToString(v)

	sig := keysign.Signature{
		Msg:        msgs[0],
		R:          rEncoded,
		S:          sEncoded,
		RecoveryID: vEncoded,
	}

	return keysign.Response{Signatures: []keysign.Signature{sig}, Status: common.Success}, nil
}

func (tm *TssMock) KeyGen(keys []string, blockHeight int64, version string) (keygen.Response, error) {
	if tm.keygenSuccess {
		sk := secp256k1.GenPrivKey()
		pk := legacybech32.MustMarshalPubKey(legacybech32.AccPK, sk.PubKey()) // nolint
		address, err := types.AccAddressFromHex(sk.PubKey().Address().String())
		if err != nil {
			panic(err)
		}
		return keygen.NewResponse(pk, address.String(), common.Success, blame.Blame{}), nil
	}
	return keygen.NewResponse("", "", common.Fail, blame.Blame{}), nil
}

func (tm *TssMock) GetTssNodeID() string {
	return "mock"
}

func (tm *TssMock) Stop() {
}
