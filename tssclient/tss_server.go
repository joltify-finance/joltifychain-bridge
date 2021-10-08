package tssclient

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"invoicebridge/config"
	"os"
	"path"

	golog "github.com/ipfs/go-log"

	"github.com/joltgeorge/tss/common"
	"github.com/libp2p/go-libp2p-peerstore/addr"

	"github.com/joltgeorge/tss/tss"

	"github.com/tendermint/tendermint/crypto/ed25519"
)

func StartTssServer(baseFolder string, tssConfig config.TssConfig) (*tss.TssServer, error) {
	golog.SetAllLoggers(golog.LevelInfo)
	_ = golog.SetLogLevel("tss-lib", "INFO")

	filePath := path.Join(baseFolder, "priv_validator_key.json")
	fmt.Printf(">>>>>%v\n", filePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("unable to read the file, invalid path")
		return nil, err
	}

	var key CosPrivKey
	err = json.Unmarshal(data, &key)
	if err != nil {
		fmt.Printf("unable to unmarshal the private key file")
		return nil, err
	}
	priKeyBytes, err := base64.StdEncoding.DecodeString(key.PrivKey.Value)
	if err != nil {
		fmt.Printf("fail to decode the private key")
		return nil, err
	}

	var privKey ed25519.PrivKey
	privKey = priKeyBytes
	fmt.Printf(">>>>%v\n", privKey.Bytes())

	tssTimeConfig := common.TssConfig{
		PartyTimeout:    tssConfig.PartyTimeout,
		KeyGenTimeout:   tssConfig.KeyGenTimeout,
		KeySignTimeout:  tssConfig.KeySignTimeout,
		PreParamTimeout: tssConfig.PreParamTimeout,
	}

	// init tss module
	tssServer, err := tss.NewTss(
		addr.AddrList(tssConfig.BootstrapPeers),
		tssConfig.Port,
		privKey,
		tssConfig.RendezvousString,
		baseFolder,
		tssTimeConfig,
		nil,
		tssConfig.ExternalIP,
	)
	fmt.Printf(">>>>>>>>>>%v\n", err)
	return tssServer, err
}

func StopTssServer(ts *tss.TssServer) {
	ts.Stop()
}
