package joltifybridge

import (
	"context"
	"encoding/base64"
	"errors"
	"io/ioutil"
	"log"
	"strconv"
	"sync"

	coscrypto "github.com/cosmos/cosmos-sdk/crypto/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/joltgeorge/tss/common"
	"github.com/joltgeorge/tss/keysign"
	"github.com/tendermint/tendermint/crypto"
	tmclienthttp "github.com/tendermint/tendermint/rpc/client/http"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
	"gitlab.com/joltify/joltifychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/types/tx"
	zlog "github.com/rs/zerolog/log"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"google.golang.org/grpc"

	xauthsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
)

// NewJoltifyBridge new the instance for the joltify pub_chain
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

func (jc *JoltifyChainBridge) genSendTx(sdkMsg []sdk.Msg, accSeq, accNum uint64, tssSignMsg *tssclient.TssSignigMsg) ([]byte, string, error) {
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

	var sigV2 signing.SignatureV2
	if tssSignMsg == nil {
		sigV2 = signing.SignatureV2{
			PubKey: key.GetPubKey(),
			Data: &signing.SingleSignatureData{
				SignMode:  encCfg.TxConfig.SignModeHandler().DefaultMode(),
				Signature: nil,
			},
			Sequence: accSeq,
		}
	} else {
		pk := tssSignMsg.Pk
		cPk, err := sdk.GetPubKeyFromBech32(sdk.Bech32PubKeyTypeAccPub, pk)
		if err != nil {
			jc.logger.Error().Err(err).Msgf("fail to get the public key from bech32 format")
			return nil, "", err
		}
		sigV2 = signing.SignatureV2{
			PubKey: cPk,
			Data: &signing.SingleSignatureData{
				SignMode:  encCfg.TxConfig.SignModeHandler().DefaultMode(),
				Signature: nil,
			},
			Sequence: accSeq,
		}

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
	signatureV2, err := jc.signTx(encCfg.TxConfig, txBuilder, signerData, tssSignMsg)
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

	// the following code is for debug only
	//txJSONBytes, err := encCfg.TxConfig.TxJSONEncoder()(txBuilder.GetTx())
	//if err != nil {
	//	fmt.Printf(">>>>fail to see the json %v", err)
	//	return nil, "", err
	//}
	//txJSON := string(txJSONBytes)
	//jc.logger.Debug().Msg(txJSON)
	return txBytes, "", nil
}

func (jc *JoltifyChainBridge) signTx(txConfig client.TxConfig, txBuilder client.TxBuilder, signerData xauthsigning.SignerData, signMsg *tssclient.TssSignigMsg) (signing.SignatureV2, error) {
	var sigV2 signing.SignatureV2

	signMode := txConfig.SignModeHandler().DefaultMode()
	// Generate the bytes to be signed.
	signBytes, err := txConfig.SignModeHandler().GetSignBytes(signMode, signerData, txBuilder.GetTx())
	if err != nil {
		return sigV2, err
	}

	var signature []byte
	var pk coscrypto.PubKey
	if signMsg == nil {
		// Sign those bytes by the node itself
		signature, pk, err = jc.keyring.Sign("operator", signBytes)
		if err != nil {
			return sigV2, err
		}
	} else {
		hashedMsg := crypto.Sha256(signBytes)
		encodedMsg := base64.StdEncoding.EncodeToString(hashedMsg)
		signMsg.Msgs = []string{encodedMsg}
		resp, err := jc.doTssSign(signMsg)
		if err != nil {
			return signing.SignatureV2{}, err
		}
		if resp.Status != common.Success {
			jc.logger.Error().Err(err).Msg("fail to generate the signature")
			// todo we need to handle the blame
			return signing.SignatureV2{}, err
		}
		if len(resp.Signatures) != 1 {
			jc.logger.Error().Msgf("we should only have 1 signature")
			return signing.SignatureV2{}, errors.New("more than 1 signature received")
		}
		signature, err = misc.SerializeSig(&resp.Signatures[0])
		if err != nil {
			jc.logger.Error().Msgf("fail to encode the signature")
			return signing.SignatureV2{}, err
		}
		pubkey, err := sdktypes.GetPubKeyFromBech32(sdk.Bech32PubKeyTypeAccPub, signMsg.Pk)
		if err != nil {
			jc.logger.Error().Err(err).Msgf("fail to get the pubkey")
			return signing.SignatureV2{}, err
		}
		pk = pubkey
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

func (jc *JoltifyChainBridge) doTssSign(msg *tssclient.TssSignigMsg) (keysign.Response, error) {
	resp, err := jc.tssServer.KeySign(msg.Pk, msg.Msgs, msg.BlockHeight, msg.Signers, tssclient.TssVersion)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to generate the tss signature")
		return keysign.Response{}, err
	}
	return resp, nil
}

// BroadcastTx broadcast the tx to the joltifyChain
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

// todo this functions current is not used
func (jc *JoltifyChainBridge) sendToken(coins sdk.Coins, from, to sdk.AccAddress, tssSignMsg *tssclient.TssSignigMsg) error {
	msg := banktypes.NewMsgSend(from, to, coins)

	acc, err := queryAccount(from.String(), jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to quer the account")
		return err
	}
	txbytes, _, err := jc.genSendTx([]sdk.Msg{msg}, acc.GetSequence(), acc.GetAccountNumber(), tssSignMsg)
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

	txbytes, _, err := jc.genSendTx([]sdk.Msg{msg}, acc.GetSequence(), acc.GetAccountNumber(), nil)
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

// GetLastBlockHeight gets the current block height
func (jc *JoltifyChainBridge) GetLastBlockHeight() (int64, error) {
	b, err := GetLastBlockHeight(jc.grpcClient)
	return b, err
}

// CheckAndUpdatePool send the tx to the joltify pub_chain, if the pool address is updated, it returns true
func (jc *JoltifyChainBridge) CheckAndUpdatePool(blockHeight int64) (bool, string) {
	jc.poolUpdateLocker.Lock()
	if len(jc.msgSendCache) < 1 {
		jc.poolUpdateLocker.Unlock()
		return false, ""
	}
	el := jc.msgSendCache[0]
	jc.poolUpdateLocker.Unlock()
	if el.blockHeight == blockHeight {
		jc.logger.Info().Msgf("we are submit the block at height>>>>>>>>%v\n", el.blockHeight)
		ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
		ok, resp, err := jc.BroadcastTx(ctx, el.data)
		if err != nil || !ok {
			jc.logger.Error().Err(err).Msgf("fail to broadcast the tx->%v", resp)
			cancel()
			return false, ""
		}
		jc.msgSendCache = jc.msgSendCache[1:]
		jc.logger.Info().Msgf("successfully broadcast the pool info")
		cancel()
		return true, el.poolPubKey
	}
	return false, ""
}

// DoTssSign test for keysign
func (jc *JoltifyChainBridge) DoTssSign() (keysign.Response, error) {
	poolInfo, err := jc.QueryLastPoolAddress()
	if err != nil {
		jc.logger.Error().Err(err).Msgf("error in get pool with error %v", err)
		return keysign.Response{}, nil

	}
	if len(poolInfo) != 2 {
		jc.logger.Info().Msgf("fail to query the pool with length %v", len(poolInfo))
		return keysign.Response{}, nil
	}
	msgtosign := base64.StdEncoding.EncodeToString([]byte("hello"))
	msg := tssclient.TssSignigMsg{
		Pk:          poolInfo[1].GetCreatePool().PoolPubKey,
		Msgs:        []string{msgtosign},
		BlockHeight: int64(100),
		Version:     tssclient.TssVersion,
	}
	resp, err := jc.doTssSign(&msg)
	return resp, err
}
