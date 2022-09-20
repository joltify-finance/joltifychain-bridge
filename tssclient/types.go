package tssclient

import (
	"github.com/oppyfinance/tss/keygen"
	"github.com/oppyfinance/tss/keysign"
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

// TssInstance is the bridge tss interface
type TssInstance interface {
	KeySign(pk string, msgs []string, blockHeight int64, signers []string, version string) (keysign.Response, error)
	KeyGen(keys []string, blockHeight int64, version string) (keygen.Response, error)
	GetTssNodeID() string
	Stop()
	// KeySign(pk string, strings []string, height int64, t interface{}, s string) (interface{}, interface{})
}

// Response key sign response
type Response interface{}
