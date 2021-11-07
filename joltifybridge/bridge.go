package joltifybridge

import (
	"context"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/tssclient"
	"gitlab.com/joltify/joltifychain/joltifychain/x/vault/types"
	"io/ioutil"
	"log"
	"strconv"
	"sync"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	tmclienthttp "github.com/tendermint/tendermint/rpc/client/http"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/types/tx"
	zlog "github.com/rs/zerolog/log"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"google.golang.org/grpc"

	xauthsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
)

//NewJoltifyBridge new the instance for the joltify pub_chain
func NewJoltifyBridge(grpcAddr, keyringPath, passcode string, config config.Config) (*JoltifyChainBridge, error) {
	var joltifyBridge JoltifyChainBridge
	var err error
	joltifyBridge.logger = zlog.With().Str("module", "joltifyChain").Logger()

	joltifyBridge.grpcClient, err = grpc.Dial(grpcAddr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	client, err := tmclienthttp.New("tcp://localhost:26657", "/websocket")
	if err != nil {
		return nil, err
	}
	err = client.Start()
	if err != nil {
		return nil, err
	}

	joltifyBridge.wsClient = client

	joltifyBridge.keyring = keyring.NewInMemory()

	dat, err := ioutil.ReadFile(keyringPath)
	if err != nil {
		log.Fatalln("error in read keyring file")
		return nil, err
	}
	err = joltifyBridge.keyring.ImportPrivKey("operator", string(dat), passcode)
	if err != nil {
		return nil, err
	}
	// fixme, in docker it needs to be changed to basehome
	tssServer, key, err := tssclient.StartTssServer(config.HomeDir, config.TssConfig)
	if err != nil {
		return nil, err
	}
	joltifyBridge.tssServer = tssServer
	joltifyBridge.cosKey = key

	joltifyBridge.msgSendCache = []tssPoolMsg{}
	joltifyBridge.LastTwoTssPoolMsg = []*tssPoolMsg{nil, nil}
	joltifyBridge.poolUpdateLocker = &sync.Mutex{}

	return &joltifyBridge, nil
}

func (jc *JoltifyChainBridge) GetTssNodeID() string {
	return jc.tssServer.GetTssNodeID()
}

func (jc *JoltifyChainBridge) TerminateBridge() error {
	err := jc.wsClient.Stop()
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to terminate the ws")
		return err
	}
	err = jc.grpcClient.Close()
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to terminate the grpc")
		return err
	}
	jc.tssServer.Stop()
	return nil
}

func (jc *JoltifyChainBridge) SendTx(sdkMsg []sdk.Msg, accSeq uint64, accNum uint64) ([]byte, string, error) {
	// Choose your codec: Amino or Protobuf. Here, we use Protobuf, given by the
	// following function.
	encCfg := MakeEncodingConfig()
	// Create a new TxBuilder.
	txBuilder := encCfg.TxConfig.NewTxBuilder()

	err := txBuilder.SetMsgs(sdkMsg...)
	if err != nil {
		return nil, "", err
	}
	// we use the default here
	txBuilder.SetGasLimit(200000)
	// txBuilder.SetFeeAmount(...)
	// txBuilder.SetMemo(...)
	// txBuilder.SetTimeoutHeight(...)

	key, err := jc.keyring.Key("operator")
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to get the operator key")
		return nil, "", err
	}

	sigV2 := signing.SignatureV2{
		PubKey: key.GetPubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  encCfg.TxConfig.SignModeHandler().DefaultMode(),
			Signature: nil,
		},
		Sequence: accSeq,
	}

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return nil, "", err
	}

	signerData := xauthsigning.SignerData{
		ChainID:       chainID,
		AccountNumber: accNum,
		Sequence:      accSeq,
	}
	signatureV2, err := jc.signTx(encCfg.TxConfig, txBuilder, signerData)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to generate the signature")
		return nil, "", err
	}
	err = txBuilder.SetSignatures(signatureV2)
	if err != nil {
		jc.logger.Error().Err(err).Msgf("fail to set the signature")
		return nil, "", err
	}

	// Generated Protobuf-encoded bytes.
	txBytes, err := encCfg.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return nil, "", err
	}

	// Generate a JSON string.
	//txJSONBytes, err := encCfg.TxConfig.TxJSONEncoder()(txBuilder.GetTx())
	//if err != nil {
	//	fmt.Printf(">>>>fail to see the json %v", err)
	//	return nil, "", err
	//}
	//txJSON := string(txJSONBytes)
	//jc.logger.Debug().Msg(txJSON)
	return txBytes, "", nil
}

func (jc *JoltifyChainBridge) signTx(txConfig client.TxConfig, txBuilder client.TxBuilder, signerData xauthsigning.SignerData) (signing.SignatureV2, error) {
	var sigV2 signing.SignatureV2

	signMode := txConfig.SignModeHandler().DefaultMode()
	// Generate the bytes to be signed.
	signBytes, err := txConfig.SignModeHandler().GetSignBytes(signMode, signerData, txBuilder.GetTx())
	if err != nil {
		return sigV2, err
	}

	// Sign those bytes
	signature, pk, err := jc.keyring.Sign("operator", signBytes)
	if err != nil {
		return sigV2, err
	}

	// Construct the SignatureV2 struct
	sigData := signing.SingleSignatureData{
		SignMode:  signMode,
		Signature: signature,
	}

	sigV2 = signing.SignatureV2{
		PubKey:   pk,
		Data:     &sigData,
		Sequence: signerData.Sequence,
	}
	return sigV2, nil
}

//BroadcastTx broadcast the tx to the joltifyChain
func (jc *JoltifyChainBridge) BroadcastTx(ctx context.Context, txBytes []byte) (bool, string, error) {
	// Broadcast the tx via gRPC. We create a new client for the Protobuf Tx
	// service.
	txClient := tx.NewServiceClient(jc.grpcClient)
	// We then call the BroadcastTx method on this client.
	grpcRes, err := txClient.BroadcastTx(
		ctx,
		&tx.BroadcastTxRequest{
			Mode:    tx.BroadcastMode_BROADCAST_MODE_SYNC,
			TxBytes: txBytes, // Proto-binary of the signed transaction, see previous step.
		},
	)
	if err != nil {
		return false, "", err
	}

	if grpcRes.GetTxResponse().Code != 0 {
		jc.logger.Error().Err(err).Msgf("fail to broadcast with response %v", grpcRes.TxResponse)
		return false, "", nil
	}
	txHash := grpcRes.GetTxResponse().TxHash
	return true, txHash, nil
}

//todo this functions current is not used
func (jc *JoltifyChainBridge) sendToken(coins sdk.Coins, from, to sdk.AccAddress) error {
	msg := banktypes.NewMsgSend(from, to, coins)

	acc, err := queryAccount(from.String(), jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to quer the account")
		return err
	}
	txbytes, _, err := jc.SendTx([]sdk.Msg{msg}, acc.GetSequence(), acc.GetAccountNumber())
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to generate the tx")
		return err
	}
	ok, _, err := jc.BroadcastTx(context.Background(), txbytes)
	if err != nil || !ok {
		jc.logger.Error().Err(err).Msg("fail to broadcast the tx")
		return err
	}
	return nil
}

func (jc *JoltifyChainBridge) prepareTssPool(creator sdk.AccAddress, pubKey, height string) error {
	msg := types.NewMsgCreateCreatePool(creator, pubKey, height)

	acc, err := queryAccount(creator.String(), jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to query the account")
		return err
	}
	txbytes, _, err := jc.SendTx([]sdk.Msg{msg}, acc.GetSequence(), acc.GetAccountNumber())
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to generate the tx")
		return err
	}

	dHeight, err := strconv.ParseInt(height, 10, 64)
	if err != nil {
		jc.logger.Error().Err(err).Msgf("fail to parse the height")
		return err
	}

	item := tssPoolMsg{
		txbytes,
		pubKey,
		dHeight,
	}
	jc.poolUpdateLocker.Lock()
	// we store the latest two tss pool address
	jc.LastTwoTssPoolMsg[0] = jc.LastTwoTssPoolMsg[1]
	jc.LastTwoTssPoolMsg[1] = &item
	jc.msgSendCache = append(jc.msgSendCache, item)
	jc.poolUpdateLocker.Unlock()
	return nil
}

//GetLastBlockHeight gets the current block height
func (jc *JoltifyChainBridge) GetLastBlockHeight() (int64, error) {
	b, err := GetLastBlockHeight(jc.grpcClient)
	return b, err
}

//TriggerSend send the tx to the joltify pub_chain, if the pool address is updated, it returns true
func (jc *JoltifyChainBridge) TriggerSend(blockHeight int64) (bool, string) {

	jc.poolUpdateLocker.Lock()
	if len(jc.msgSendCache) < 1 {
		jc.poolUpdateLocker.Unlock()
		return false, ""
	}
	el := jc.msgSendCache[0]
	jc.poolUpdateLocker.Unlock()
	if el.blockHeight == blockHeight {
		jc.logger.Info().Msgf("we are submit the block at height>>>>>>>>%v\n", el.blockHeight)
		ok, resp, err := jc.BroadcastTx(context.Background(), el.data)
		if err != nil || !ok {
			jc.logger.Error().Err(err).Msgf("fail to broadcast the tx->%v", resp)
			return false, ""
		}
		jc.msgSendCache = jc.msgSendCache[1:]
		jc.logger.Info().Msgf("successfully broadcast the pool info")
		return true, el.poolPubKey
	}
	return false, ""
}
