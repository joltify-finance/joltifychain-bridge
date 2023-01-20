package cossubmit

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sync"

	"github.com/cenkalti/backoff"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	types3 "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32"
	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	signing2 "github.com/cosmos/cosmos-sdk/x/auth/signing"
	grpc1 "github.com/gogo/protobuf/grpc"
	common2 "github.com/joltify-finance/tss/common"
	"github.com/joltify-finance/tss/keysign"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/tmhash"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
)

func (cs *CosHandler) BatchComposeAndSend(conn grpc1.ClientConn, sendMsg []types.Msg, accSeq, accNum uint64, signMsg *tssclient.TssSignigMsg, poolAddress string) (map[uint64]string, error) {
	gasWanted, err := cs.GasEstimation(conn, sendMsg, accSeq, nil)
	if err != nil {
		cs.logger.Error().Err(err).Msg("Fail to get the gas estimation")
		return nil, err
	}

	txBuilderSeqMap, err := cs.BatchGenSendTx(sendMsg, accSeq, accNum, gasWanted, signMsg)
	if err != nil {
		cs.logger.Error().Err(err).Msg("fail to generate the tx")
		return nil, err
	}

	wg := sync.WaitGroup{}
	txHashes := make(map[uint64]string)
	txHashesLocker := &sync.RWMutex{}
	for seq, el := range txBuilderSeqMap {
		if el == nil {
			cs.logger.Error().Msgf("the seq %v has nil tx builder!!", seq)
			txHashes[seq] = ""
			continue
		}
		wg.Add(1)
		go func(accSeq uint64, txBuilder client.TxBuilder) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
			defer cancel()

			txBytes, err := cs.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
			if err != nil {
				cs.logger.Error().Err(err).Msg("fail to encode the tx")
				txHashesLocker.Lock()
				txHashes[accSeq] = ""
				txHashesLocker.Unlock()
				return
			}
			err = cs.waitAndSend(conn, poolAddress, accSeq)
			if err == nil {
				_, resp, err := cs.BroadcastTx(ctx, conn, txBytes, true)
				if err != nil {
					cs.logger.Error().Err(err).Msg("fail to broadcast the signature")
				}
				txHashesLocker.Lock()
				txHashes[accSeq] = resp
				txHashesLocker.Unlock()
				return
			}
		}(seq, el)
	}
	wg.Wait()
	return txHashes, nil
}

func (cs *CosHandler) ComposeAndSend(conn grpc1.ClientConn, operator keyring.Info, sendMsg types.Msg, accSeq, accNum uint64, signMsg *tssclient.TssSignigMsg, poolAddress string) (bool, string, error) {
	gasWanted, err := cs.GasEstimation(conn, []types.Msg{sendMsg}, accSeq, nil)
	if err != nil {
		cs.logger.Error().Err(err).Msg("Fail to get the gas estimation")
		return false, "", err
	}

	txBuilder, err := cs.GenSendTx(operator, []types.Msg{sendMsg}, accSeq, accNum, gasWanted, signMsg)
	if err != nil {
		cs.logger.Error().Err(err).Msg("fail to generate the tx")
		return false, "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	txBytes, err := cs.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		cs.logger.Error().Err(err).Msg("fail to encode the tx")
		return false, "", err
	}

	err = nil
	if signMsg != nil {
		err = cs.waitAndSend(conn, poolAddress, accSeq)
	}
	if err == nil {
		isTssMsg := true
		if signMsg == nil {
			isTssMsg = false
		}
		ok, resp, err := cs.BroadcastTx(ctx, conn, txBytes, isTssMsg)
		return ok, resp, err
	}
	return false, "", err
}

// BroadcastTx broadcast the tx to the oppyChain
func (cs *CosHandler) BroadcastTx(ctx context.Context, conn grpc1.ClientConn, txBytes []byte, isTssMsg bool) (bool, string, error) {
	// Broadcast the tx via gRPC. We create a new client for the Protobuf Tx
	// service.
	txClient := tx.NewServiceClient(conn)
	// We then call the BroadcastTx method on this client.
	grpcRes, err := txClient.BroadcastTx(
		ctx,
		&tx.BroadcastTxRequest{
			Mode:    tx.BroadcastMode_BROADCAST_MODE_BLOCK,
			TxBytes: txBytes, // Proto-binary of the signed transaction, see previous step.
		},
	)
	if err != nil {
		return false, hex.EncodeToString(tmhash.Sum(txBytes)), err
	}

	txHash := grpcRes.GetTxResponse().TxHash

	// this mean tx has been submitted by others
	if grpcRes.GetTxResponse().Code == 19 {
		return true, txHash, nil
	}

	if grpcRes.GetTxResponse().Code != 0 {
		cs.logger.Error().Err(err).Msgf("fail to broadcast with response %v", grpcRes.TxResponse)
		return false, "", nil
	}

	return true, txHash, nil
}

// CheckIssueTokenTxStatus check whether the tx has been done successfully
func (cs *CosHandler) waitAndSend(conn grpc1.ClientConn, senderAddress string, targetSeq uint64) error {
	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(submitBackoff), 40)

	alreadyPassed := false
	op := func() error {
		acc, err := common.QueryAccount(conn, senderAddress, cs.GrpcAddr)
		if err != nil {
			cs.logger.Error().Err(err).Msgf("fail to query the Account")
			return errors.New("invalid Account query")
		}
		if acc.GetSequence() == targetSeq {
			return nil
		}
		if acc.GetSequence() > targetSeq {
			alreadyPassed = true
			return nil
		}
		return errors.New("not our round")
	}

	err := backoff.Retry(op, bf)
	if alreadyPassed {
		return errors.New("already passed")
	}
	return err
}

func (cs *CosHandler) BatchGenSendTx(sdkMsg []types.Msg, accSeq, accNum, gasWanted uint64, tssSignMsg *tssclient.TssSignigMsg) (map[uint64]client.TxBuilder, error) {
	// Choose your codec: Amino or Protobuf. Here, we use Protobuf, given by the
	// following function.
	pubkey, err := legacybech32.UnmarshalPubKey(legacybech32.AccPK, tssSignMsg.Pk) //nolint
	if err != nil {
		cs.logger.Error().Err(err).Msgf("fail to get the pubkey")
		return nil, err
	}

	encCfg := *cs.encoding
	var tssSignRawMsgs []string
	txBuilderMap := make(map[string]client.TxBuilder)
	unSignedSigMap := make(map[string]*signing.SignatureV2)
	txBuilderSeqMap := make(map[uint64]client.TxBuilder)
	for i, eachMsg := range sdkMsg {
		// Create a new TxBuilder.
		txBuilder := encCfg.TxConfig.NewTxBuilder()
		err := txBuilder.SetMsgs(eachMsg)
		if err != nil {
			return nil, err
		}
		// we use the default here
		txBuilder.SetGasLimit(gasWanted)
		// txBuilder.SetFeeAmount(...)
		// txBuilder.SetMemo(...)
		// txBuilder.SetTimeoutHeight(...)
		var sigV2 signing.SignatureV2

		pk := tssSignMsg.Pk
		cPk, err := legacybech32.UnmarshalPubKey(legacybech32.AccPK, pk) //nolint
		if err != nil {
			cs.logger.Error().Err(err).Msgf("fail to get the public key from bech32 format")
			return nil, err
		}
		sigV2 = signing.SignatureV2{
			PubKey: cPk,
			Data: &signing.SingleSignatureData{
				SignMode:  encCfg.TxConfig.SignModeHandler().DefaultMode(),
				Signature: nil,
			},
			Sequence: accSeq + uint64(i),
		}

		err = txBuilder.SetSignatures(sigV2)
		if err != nil {
			cs.logger.Error().Err(err).Msgf("fail to build the signature")
			continue
		}

		signMode := encCfg.TxConfig.SignModeHandler().DefaultMode()

		signerData := signing2.SignerData{
			ChainID:       cs.ChainId,
			AccountNumber: accNum,
			Sequence:      accSeq + uint64(i),
		}

		// Generate the bytes to be signed.
		signBytes, err := encCfg.TxConfig.SignModeHandler().GetSignBytes(signMode, signerData, txBuilder.GetTx())
		if err != nil {
			cs.logger.Error().Err(err).Msgf("fail to build the signature")
			continue
		}

		hashedMsg := crypto.Sha256(signBytes)
		encodedMsg := base64.StdEncoding.EncodeToString(hashedMsg)
		tssSignRawMsgs = append(tssSignRawMsgs, encodedMsg)
		txBuilderMap[encodedMsg] = txBuilder
		unSignedSigMap[encodedMsg] = &sigV2
	}

	tssSignMsg.Msgs = tssSignRawMsgs
	resp, err := cs.doTssSign(tssSignMsg)
	if err != nil {
		return nil, err
	}
	if resp.Status != common2.Success {
		cs.logger.Error().Err(err).Msg("fail to generate the signature")
		// todo we need to handle the blame
		return nil, err
	}
	if len(resp.Signatures) != len(tssSignRawMsgs) {
		cs.logger.Error().Msgf("the signature and msg to be signed mismathch")
		return nil, errors.New("more than 1 signature received")
	}

	for _, el := range resp.Signatures {
		each := el
		thisSignature, err := misc.SerializeSig(&each, false)
		if err != nil {
			cs.logger.Error().Msgf("fail to encode the signature")
			continue
		}

		txBuilder := txBuilderMap[el.Msg]
		unSignedSig := unSignedSigMap[el.Msg]
		// Construct the SignatureV2 struct
		sigData := signing.SingleSignatureData{
			SignMode:  encCfg.TxConfig.SignModeHandler().DefaultMode(),
			Signature: thisSignature,
		}

		signedSigV2 := signing.SignatureV2{
			PubKey:   pubkey,
			Data:     &sigData,
			Sequence: unSignedSig.Sequence,
		}

		err = txBuilder.SetSignatures(signedSigV2)
		if err != nil {
			cs.logger.Error().Err(err).Msgf("fail to set the signature")
			txBuilderSeqMap[unSignedSig.Sequence] = nil
		}
		txBuilderSeqMap[unSignedSig.Sequence] = txBuilder
	}
	return txBuilderSeqMap, nil
}

func (cs *CosHandler) GenSendTx(key keyring.Info, sdkMsg []types.Msg, accSeq, accNum, gasWanted uint64, tssSignMsg *tssclient.TssSignigMsg) (client.TxBuilder, error) {
	// Choose your codec: Amino or Protobuf. Here, we use Protobuf, given by the
	// following function.
	encCfg := *cs.encoding
	// Create a new TxBuilder.
	txBuilder := encCfg.TxConfig.NewTxBuilder()

	err := txBuilder.SetMsgs(sdkMsg...)
	if err != nil {
		return nil, err
	}

	// we use the default here
	txBuilder.SetGasLimit(gasWanted)
	// txBuilder.SetFeeAmount(...)
	// txBuilder.SetMemo(...)
	// txBuilder.SetTimeoutHeight(...)

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
		cPk, err := legacybech32.UnmarshalPubKey(legacybech32.AccPK, pk) //nolint
		if err != nil {
			cs.logger.Error().Err(err).Msgf("fail to get the public key from bech32 format")
			return nil, err
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
		return nil, err
	}

	signerData := signing2.SignerData{
		ChainID:       cs.ChainId,
		AccountNumber: accNum,
		Sequence:      accSeq,
	}
	signatureV2, err := cs.signTx(encCfg.TxConfig, txBuilder, signerData, tssSignMsg)
	if err != nil {
		cs.logger.Error().Err(err).Msg("fail to generate the signature")
		return nil, err
	}
	err = txBuilder.SetSignatures(signatureV2)
	if err != nil {
		cs.logger.Error().Err(err).Msgf("fail to set the signature")
		return nil, err
	}

	return txBuilder, nil
}

func (cs *CosHandler) signTx(txConfig client.TxConfig, txBuilder client.TxBuilder, signerData signing2.SignerData, signMsg *tssclient.TssSignigMsg) (signing.SignatureV2, error) {
	var sigV2 signing.SignatureV2

	signMode := txConfig.SignModeHandler().DefaultMode()
	// Generate the bytes to be signed.
	signBytes, err := txConfig.SignModeHandler().GetSignBytes(signMode, signerData, txBuilder.GetTx())
	if err != nil {
		return sigV2, err
	}

	var signature []byte
	var pk types3.PubKey
	if signMsg == nil {
		// Sign those bytes by the node itself
		signature, pk, err = cs.Keyring.Sign("operator", signBytes)
		if err != nil {
			return sigV2, err
		}
	} else {
		hashedMsg := crypto.Sha256(signBytes)
		encodedMsg := base64.StdEncoding.EncodeToString(hashedMsg)
		signMsg.Msgs = []string{encodedMsg}
		resp, err := cs.doTssSign(signMsg)
		if err != nil {
			return signing.SignatureV2{}, err
		}
		if resp.Status != common2.Success {
			cs.logger.Error().Err(err).Msg("fail to generate the signature")
			// todo we need to handle the blame
			return signing.SignatureV2{}, err
		}
		if len(resp.Signatures) != 1 {
			cs.logger.Error().Msgf("we should only have 1 signature")
			return signing.SignatureV2{}, errors.New("more than 1 signature received")
		}
		signature, err = misc.SerializeSig(&resp.Signatures[0], false)
		if err != nil {
			cs.logger.Error().Msgf("fail to encode the signature")
			return signing.SignatureV2{}, err
		}

		pubkey, err := legacybech32.UnmarshalPubKey(legacybech32.AccPK, signMsg.Pk) //nolint
		if err != nil {
			cs.logger.Error().Err(err).Msgf("fail to get the pubkey")
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

func (cs *CosHandler) doTssSign(msg *tssclient.TssSignigMsg) (keysign.Response, error) {
	resp, err := cs.tssServer.KeySign(msg.Pk, msg.Msgs, msg.BlockHeight, msg.Signers, tssclient.TssVersion)
	if err != nil {
		cs.logger.Error().Err(err).Msg("fail to generate the tss signature")
		return keysign.Response{}, err
	}
	return resp, nil
}

// CheckIssueTokenTxStatus check whether the tx has been done successfully
func (cs *CosHandler) CheckIssueTokenTxStatus(conn grpc1.ClientConn, index string, retryNum uint64) error {
	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(submitBackoff), retryNum)

	op := func() error {
		if cs.CheckWhetherIssueTokenAlreadyExist(conn, index) {
			return nil
		}
		return errors.New("fail to find the tx")
	}

	err := backoff.Retry(op, bf)
	return err
}

func (cs *CosHandler) QueryTxStatus(grpcClient grpc1.ClientConn, txHash string, retryNum uint64) error {
	if len(txHash) == 0 {
		return errors.New("empty txhash for query")
	}
	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(submitBackoff), retryNum)
	txHashB, err := hex.DecodeString(txHash)
	if err != nil {
		cs.logger.Error().Err(err).Msgf("fail to decode %v", txHash)
		panic("invalid tx hash")
	}
	op := func() error {
		t, err := common.GetGivenTx(grpcClient, txHashB)
		if err != nil {
			cs.logger.Error().Err(err).Msgf("fail to query the tx with given hash %v", txHash)
		}
		if err == nil && t.TxResponse.Code == 0 {
			return nil
		}

		return errors.New("fail to find the tx or tx status code is not 0")
	}

	err = backoff.Retry(op, bf)
	return err
}

func (cs *CosHandler) GetCurrentNewBlockChain() <-chan ctypes.ResultEvent {
	return cs.currentNewBlockChan
}

func (cs *CosHandler) GetChannelQueueNewBlockChain() chan ctypes.ResultEvent {
	return cs.ChannelQueueNewBlock
}

func (cs *CosHandler) GetCurrentNewValidator() <-chan ctypes.ResultEvent {
	return cs.CurrentNewValidator
}

func (cs *CosHandler) GetChannelQueueNewValidator() chan ctypes.ResultEvent {
	return cs.ChannelQueueValidator
}
