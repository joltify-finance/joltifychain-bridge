package bridge

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

func NewInvoiceBridge(grpcAddr, keyringPath, passcode string, config config.Config) (*InvChainBridge, error) {
	var invoiceBridge InvChainBridge
	var err error
	invoiceBridge.logger = zlog.With().Str("module", "joltifyChain").Logger()

	invoiceBridge.grpcClient, err = grpc.Dial(grpcAddr, grpc.WithInsecure())
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

	invoiceBridge.wsClient = client

	invoiceBridge.keyring = keyring.NewInMemory()

	dat, err := ioutil.ReadFile(keyringPath)
	if err != nil {
		log.Fatalln("error in read keyring file")
		return nil, err
	}
	err = invoiceBridge.keyring.ImportPrivKey("operator", string(dat), passcode)
	if err != nil {
		return nil, err
	}
	// fixme, in docker it needs to be changed to basehome

	tssServer, key, err := tssclient.StartTssServer(config.HomeDir, config.TssConfig)
	if err != nil {
		return nil, err
	}
	invoiceBridge.tssServer = tssServer
	invoiceBridge.cosKey = key

	invoiceBridge.msgSendCache = []TssPoolMsg{}
	invoiceBridge.LastTwoTssPoolMsg = []*TssPoolMsg{nil, nil}
	invoiceBridge.poolUpdateLocker = &sync.Mutex{}

	return &invoiceBridge, nil
}

func (ic *InvChainBridge) GetTssNodeID() string {
	return ic.tssServer.GetTssNodeID()
}

func (ic *InvChainBridge) TerminateBridge() error {
	err := ic.wsClient.Stop()
	if err != nil {
		ic.logger.Error().Err(err).Msg("fail to terminate the ws")
		return err
	}
	err = ic.grpcClient.Close()
	if err != nil {
		ic.logger.Error().Err(err).Msg("fail to terminate the grpc")
		return err
	}
	ic.tssServer.Stop()
	return nil
}

func (ic *InvChainBridge) SendTx(sdkMsg []sdk.Msg, accSeq uint64, accNum uint64) ([]byte, string, error) {
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

	key, err := ic.keyring.Key("operator")
	if err != nil {
		ic.logger.Error().Err(err).Msg("fail to get the operator key")
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
	signatureV2, err := ic.signTx(encCfg.TxConfig, txBuilder, signerData)
	if err != nil {
		ic.logger.Error().Err(err).Msg("fail to generate the signature")
		return nil, "", err
	}
	err = txBuilder.SetSignatures(signatureV2)
	if err != nil {
		ic.logger.Error().Err(err).Msgf("fail to set the signature")
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
	//ic.logger.Debug().Msg(txJSON)
	return txBytes, "", nil
}

func (ic *InvChainBridge) signTx(txConfig client.TxConfig, txBuilder client.TxBuilder, signerData xauthsigning.SignerData) (signing.SignatureV2, error) {
	var sigV2 signing.SignatureV2

	signMode := txConfig.SignModeHandler().DefaultMode()
	// Generate the bytes to be signed.
	signBytes, err := txConfig.SignModeHandler().GetSignBytes(signMode, signerData, txBuilder.GetTx())
	if err != nil {
		return sigV2, err
	}

	// Sign those bytes
	signature, pk, err := ic.keyring.Sign("operator", signBytes)
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

func (ic *InvChainBridge) BroadcastTx(ctx context.Context, txBytes []byte) (bool, string, error) {
	// Broadcast the tx via gRPC. We create a new client for the Protobuf Tx
	// service.
	txClient := tx.NewServiceClient(ic.grpcClient)
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
		ic.logger.Error().Err(err).Msgf("fail to broadcast with response %v", grpcRes.TxResponse)
		return false, "", nil
	}
	txHash := grpcRes.GetTxResponse().TxHash
	return true, txHash, nil
}

func (ic *InvChainBridge) SendToken(coins sdk.Coins, from, to sdk.AccAddress) error {
	msg := banktypes.NewMsgSend(from, to, coins)

	acc, err := QueryAccount(from.String(), ic.grpcClient)
	if err != nil {
		ic.logger.Error().Err(err).Msg("Fail to quer the account")
		return err
	}
	txbytes, _, err := ic.SendTx([]sdk.Msg{msg}, acc.GetSequence(), acc.GetAccountNumber())
	if err != nil {
		ic.logger.Error().Err(err).Msg("fail to generate the tx")
		return err
	}
	ok, _, err := ic.BroadcastTx(context.Background(), txbytes)
	if err != nil || !ok {
		ic.logger.Error().Err(err).Msg("fail to broadcast the tx")
		return err
	}
	return nil
}

func (ic *InvChainBridge) PrepareTssPool(creator sdk.AccAddress, pubKey, height string) error {
	msg := types.NewMsgCreateCreatePool(creator, pubKey, height)

	acc, err := QueryAccount(creator.String(), ic.grpcClient)
	if err != nil {
		ic.logger.Error().Err(err).Msg("Fail to query the account")
		return err
	}
	txbytes, _, err := ic.SendTx([]sdk.Msg{msg}, acc.GetSequence(), acc.GetAccountNumber())
	if err != nil {
		ic.logger.Error().Err(err).Msg("fail to generate the tx")
		return err
	}

	dHeight, err := strconv.ParseInt(height, 10, 64)
	if err != nil {
		ic.logger.Error().Err(err).Msgf("fail to parse the height")
		return err
	}

	item := TssPoolMsg{
		txbytes,
		dHeight,
	}
	ic.poolUpdateLocker.Lock()
	// we store the latest two tss pool address
	ic.LastTwoTssPoolMsg[0] = ic.LastTwoTssPoolMsg[1]
	ic.LastTwoTssPoolMsg[1] = &item
	ic.msgSendCache = append(ic.msgSendCache, item)
	ic.poolUpdateLocker.Unlock()
	return nil
}

func (ic *InvChainBridge) GetLastBlockHeight() (int64, error) {
	b, err := GetLastBlockHeight(ic.grpcClient)
	return b, err
}

func (ic *InvChainBridge) TriggerSend(blockHeight int64) {

	ic.poolUpdateLocker.Lock()
	if len(ic.msgSendCache) < 1 {
		ic.poolUpdateLocker.Unlock()
		return
	}
	el := ic.msgSendCache[0]
	ic.poolUpdateLocker.Unlock()
	if el.blockHeight == blockHeight {
		ic.logger.Info().Msgf("we are submit the block at height>>>>>>>>%v\n", el.blockHeight)
		ok, resp, err := ic.BroadcastTx(context.Background(), el.data)
		if err != nil || !ok {
			ic.logger.Error().Err(err).Msgf("fail to broadcast the tx->%v", resp)
			return
		}
		ic.msgSendCache = ic.msgSendCache[1:]
		ic.logger.Info().Msgf("successfully broadcast the pool info")
		return
	}

}
