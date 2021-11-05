package bridge

import (
	"encoding/base64"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
)

func ImportPrivKey(privkey string) (cryptotypes.PrivKey, error) {
	privkeyBytes, err := base64.StdEncoding.DecodeString(privkey)
	if err != nil {
		return nil, err
	}

	//var ret cryptotypes.PrivKey
	ret := &ed25519.PrivKey{
		Key: privkeyBytes,
	}
	return ret, nil

}
