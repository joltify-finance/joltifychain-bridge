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
