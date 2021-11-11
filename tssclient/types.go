package tssclient

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
