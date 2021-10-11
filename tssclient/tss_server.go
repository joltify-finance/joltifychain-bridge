package tssclient

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"invoicebridge/config"
	"os"
	"path"

	"github.com/joltgeorge/tss/keygen"

	golog "github.com/ipfs/go-log"

	"github.com/joltgeorge/tss/common"
	"github.com/libp2p/go-libp2p-peerstore/addr"

	tsslib "github.com/joltgeorge/tss/tss"

	"github.com/tendermint/tendermint/crypto/ed25519"
)

const Version = "0.14.0"

type BridgeTssServer struct {
	ts *tsslib.TssServer
}

func StartTssServer(baseFolder string, tssConfig config.TssConfig) (*BridgeTssServer, CosPrivKey, error) {
	golog.SetAllLoggers(golog.LevelInfo)
	_ = golog.SetLogLevel("tss-lib", "INFO")

	filePath := path.Join(baseFolder, "priv_validator_key.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("unable to read the file, invalid path")
		return nil, CosPrivKey{}, err
	}

	var key CosPrivKey
	err = json.Unmarshal(data, &key)
	if err != nil {
		fmt.Printf("unable to unmarshal the private key file")
		return nil, CosPrivKey{}, err
	}
	priKeyBytes, err := base64.StdEncoding.DecodeString(key.PrivKey.Value)
	if err != nil {
		fmt.Printf("fail to decode the private key")
		return nil, CosPrivKey{}, err
	}

	var privKey ed25519.PrivKey
	privKey = priKeyBytes

	tssTimeConfig := common.TssConfig{
		PartyTimeout:    tssConfig.PartyTimeout,
		KeyGenTimeout:   tssConfig.KeyGenTimeout,
		KeySignTimeout:  tssConfig.KeySignTimeout,
		PreParamTimeout: tssConfig.PreParamTimeout,
	}

	// init tss module
	ts, err := tsslib.NewTss(
		addr.AddrList(tssConfig.BootstrapPeers),
		tssConfig.Port,
		privKey,
		tssConfig.RendezvousString,
		baseFolder,
		tssTimeConfig,
		nil,
		tssConfig.ExternalIP,
	)

	tc := BridgeTssServer{
		ts,
	}

	return &tc, key, err
}

func (tc *BridgeTssServer) Keygen(keys []string, blockHeight int64, version string) (keygen.Response, error) {
	req := keygen.NewRequest(keys, blockHeight, version)
	resp, err := tc.ts.Keygen(req)
	if err != nil {
		return keygen.Response{}, err
	}
	return resp, nil
}

func (tc *BridgeTssServer) GetTssNodeID() string {
	return tc.ts.GetLocalPeerID()
}

func (tc *BridgeTssServer) Stop() {
	tc.ts.Stop()
}
