package tssclient

import (
	"github.com/joltgeorge/tss/keygen"
	"github.com/joltgeorge/tss/keysign"
)

type CosPrivKey struct {
	Address string `json:"address"`
	PubKey  struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"pub_key"`
	PrivKey struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"priv_key"`
}

// TssSignigMsg is the packed message for tss signing
type TssSignigMsg struct {
	Pk          string
	Msgs        []string
	Signers     []string
	BlockHeight int64
	Version     string
}

type TssSign interface {
	KeySign(pk string, msgs []string, blockHeight int64, signers []string, version string) (keysign.Response, error)
	KeyGen(keys []string, blockHeight int64, version string) (keygen.Response, error)
	GetTssNodeID() string
}
