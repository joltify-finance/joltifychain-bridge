package cosbridge

import (
	"crypto/ecdsa"
	"encoding/base64"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/joltify-finance/tss/blame"
	tcommon "github.com/joltify-finance/tss/common"
	"github.com/joltify-finance/tss/keygen"
	"github.com/joltify-finance/tss/keysign"
)

type Account struct {
	sk       *secp256k1.PrivKey
	pk       string
	joltAddr types.AccAddress
	commAddr common.Address
}

func generateRandomPrivKey(n int) ([]Account, error) {
	randomAccounts := make([]Account, n)
	for i := 0; i < n; i++ {
		sk := secp256k1.GenPrivKey()
		pk := legacybech32.MustMarshalPubKey(legacybech32.AccPK, sk.PubKey()) // nolint

		ethAddr, err := misc.PoolPubKeyToEthAddress(pk)
		if err != nil {
			return nil, err
		}
		addrOppy, err := types.AccAddressFromHex(sk.PubKey().Address().String())
		if err != nil {
			return nil, err
		}
		tAccount := Account{
			sk,
			pk,
			addrOppy,
			ethAddr,
		}
		randomAccounts[i] = tAccount
	}
	return randomAccounts, nil
}

type TssMock struct {
	sk             *secp256k1.PrivKey
	keys           keyring.Keyring
	keygenSuccess  bool
	keysignSuccess bool
}

func (tm *TssMock) KeySign(pk string, msgs []string, blockHeight int64, signers []string, version string) (keysign.Response, error) {
	if !tm.keysignSuccess {
		return keysign.NewResponse(nil, tcommon.Fail, blame.NewBlame("", nil)), errors.New("fail in keysign")
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

	return keysign.Response{Signatures: []keysign.Signature{sig}, Status: tcommon.Success}, nil
}

func (tm *TssMock) KeyGen(_ []string, _ int64, _ string) (keygen.Response, error) {
	if tm.keygenSuccess {
		sk := secp256k1.GenPrivKey()
		pk := legacybech32.MustMarshalPubKey(legacybech32.AccPK, sk.PubKey()) // nolint
		address, err := types.AccAddressFromHex(sk.PubKey().Address().String())
		if err != nil {
			panic(err)
		}
		return keygen.NewResponse(pk, address.String(), tcommon.Success, blame.Blame{}), nil
	}
	return keygen.NewResponse("", "", tcommon.Fail, blame.Blame{}), nil
}

func (tm *TssMock) GetTssNodeID() string {
	return "mock"
}

func (tm *TssMock) Stop() {
}
