package tssclient

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/joltify-finance/tss/keysign"
	"gitlab.com/joltify/joltifychain-bridge/config"

	"github.com/joltify-finance/tss/keygen"

	golog "github.com/ipfs/go-log"

	"github.com/joltify-finance/tss/common"
	"github.com/libp2p/go-libp2p-peerstore/addr"

	tsslib "github.com/joltify-finance/tss/tss"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/tendermint/tendermint/crypto/ed25519"
)

// TssVersion that we apply to
const TssVersion = "0.14.0"

// BridgeTssServer the entity of tss server
type BridgeTssServer struct {
	ts *tsslib.TssServer
}

// StartTssServer start the tss server
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

	var privKey ed25519.PrivKey = priKeyBytes

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

// KeyGen generate the tss key
func (tc *BridgeTssServer) KeyGen(keys []string, blockHeight int64, version string) (keygen.Response, error) {
	req := keygen.NewRequest(keys, blockHeight, version)
	resp, err := tc.ts.Keygen(req)
	if err != nil {
		return keygen.Response{}, err
	}
	return resp, nil
}

// KeySign generates the signature
func (tc *BridgeTssServer) KeySign(pk string, msgs []string, blockHeight int64, signers []string, version string) (keysign.Response, error) {
	req := keysign.NewRequest(pk, msgs, blockHeight, signers, version)
	resp, err := tc.ts.KeySign(req)
	if err != nil {
		return keysign.Response{}, err
	}
	return resp, nil
}

// GetTssNodeID get the tss node ID
func (tc *BridgeTssServer) GetTssNodeID() string {
	ret, err := peer.IDFromString(tc.ts.GetLocalPeerID())
	if err != nil {
		return ""
	}
	fmt.Printf(">>>>>>%v\n", ret)
	return string(ret)
}

// Stop stop the tss server
func (tc *BridgeTssServer) Stop() {
	tc.ts.Stop()
}
