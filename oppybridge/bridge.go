package oppybridge

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" //nolint
	cosTx "github.com/cosmos/cosmos-sdk/types/tx"
	ethcommon "github.com/ethereum/go-ethereum/common"
	grpc1 "github.com/gogo/protobuf/grpc"
	prototypes "github.com/tendermint/tendermint/proto/tendermint/types"
	tendertypes "github.com/tendermint/tendermint/types"
	bcommon "gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/config"

	coscrypto "github.com/cosmos/cosmos-sdk/crypto/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/oppyfinance/tss/common"
	"github.com/oppyfinance/tss/keysign"
	"github.com/tendermint/tendermint/crypto"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
	"gitlab.com/oppy-finance/oppy-bridge/tssclient"
	"gitlab.com/oppy-finance/oppychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	zlog "github.com/rs/zerolog/log"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	xauthsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
)

func (oc *OppyChainInstance) GetTssNodeID() string {
	return oc.tssServer.GetTssNodeID()
}

func (oc *OppyChainInstance) TerminateBridge() error {
	err := oc.WsClient.Stop()
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to terminate the ws")
		return err
	}
	oc.tssServer.Stop()
	return nil
}

func (oc *OppyChainInstance) batchGenSendTx(sdkMsg []sdk.Msg, accSeq, accNum, gasWanted uint64, tssSignMsg *tssclient.TssSignigMsg) (map[uint64]client.TxBuilder, error) {
	// Choose your codec: Amino or Protobuf. Here, we use Protobuf, given by the
	// following function.
	pubkey, err := legacybech32.UnmarshalPubKey(legacybech32.AccPK, tssSignMsg.Pk) //nolint
	if err != nil {
		oc.logger.Error().Err(err).Msgf("fail to get the pubkey")
		return nil, err
	}

	encCfg := *oc.encoding
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
			oc.logger.Error().Err(err).Msgf("fail to get the public key from bech32 format")
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
			oc.logger.Error().Err(err).Msgf("fail to build the signature")
			continue
		}

		signMode := encCfg.TxConfig.SignModeHandler().DefaultMode()

		signerData := xauthsigning.SignerData{
			ChainID:       config.ChainID,
			AccountNumber: accNum,
			Sequence:      accSeq + uint64(i),
		}

		// Generate the bytes to be signed.
		signBytes, err := encCfg.TxConfig.SignModeHandler().GetSignBytes(signMode, signerData, txBuilder.GetTx())
		if err != nil {
			oc.logger.Error().Err(err).Msgf("fail to build the signature")
			continue
		}

		hashedMsg := crypto.Sha256(signBytes)
		encodedMsg := base64.StdEncoding.EncodeToString(hashedMsg)
		tssSignRawMsgs = append(tssSignRawMsgs, encodedMsg)
		txBuilderMap[encodedMsg] = txBuilder
		unSignedSigMap[encodedMsg] = &sigV2
	}

	tssSignMsg.Msgs = tssSignRawMsgs
	resp, err := oc.doTssSign(tssSignMsg)
	if err != nil {
		return nil, err
	}
	if resp.Status != common.Success {
		oc.logger.Error().Err(err).Msg("fail to generate the signature")
		// todo we need to handle the blame
		return nil, err
	}
	if len(resp.Signatures) != len(tssSignRawMsgs) {
		oc.logger.Error().Msgf("the signature and msg to be signed mismathch")
		return nil, errors.New("more than 1 signature received")
	}

	for _, el := range resp.Signatures {
		each := el
		thisSignature, err := misc.SerializeSig(&each, false)
		if err != nil {
			oc.logger.Error().Msgf("fail to encode the signature")
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
			oc.logger.Error().Err(err).Msgf("fail to set the signature")
			txBuilderSeqMap[unSignedSig.Sequence] = nil
		}
		txBuilderSeqMap[unSignedSig.Sequence] = txBuilder
	}
	return txBuilderSeqMap, nil
}

func (oc *OppyChainInstance) genSendTx(key keyring.Info, sdkMsg []sdk.Msg, accSeq, accNum, gasWanted uint64, tssSignMsg *tssclient.TssSignigMsg) (client.TxBuilder, error) {
	// Choose your codec: Amino or Protobuf. Here, we use Protobuf, given by the
	// following function.
	encCfg := *oc.encoding
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
			oc.logger.Error().Err(err).Msgf("fail to get the public key from bech32 format")
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

	signerData := xauthsigning.SignerData{
		ChainID:       config.ChainID,
		AccountNumber: accNum,
		Sequence:      accSeq,
	}
	signatureV2, err := oc.signTx(encCfg.TxConfig, txBuilder, signerData, tssSignMsg)
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to generate the signature")
		return nil, err
	}
	err = txBuilder.SetSignatures(signatureV2)
	if err != nil {
		oc.logger.Error().Err(err).Msgf("fail to set the signature")
		return nil, err
	}

	return txBuilder, nil
}

func (oc *OppyChainInstance) signTx(txConfig client.TxConfig, txBuilder client.TxBuilder, signerData xauthsigning.SignerData, signMsg *tssclient.TssSignigMsg) (signing.SignatureV2, error) {
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
		signature, pk, err = oc.Keyring.Sign("operator", signBytes)
		if err != nil {
			return sigV2, err
		}
	} else {
		hashedMsg := crypto.Sha256(signBytes)
		encodedMsg := base64.StdEncoding.EncodeToString(hashedMsg)
		signMsg.Msgs = []string{encodedMsg}
		resp, err := oc.doTssSign(signMsg)
		if err != nil {
			return signing.SignatureV2{}, err
		}
		if resp.Status != common.Success {
			oc.logger.Error().Err(err).Msg("fail to generate the signature")
			// todo we need to handle the blame
			return signing.SignatureV2{}, err
		}
		if len(resp.Signatures) != 1 {
			oc.logger.Error().Msgf("we should only have 1 signature")
			return signing.SignatureV2{}, errors.New("more than 1 signature received")
		}
		signature, err = misc.SerializeSig(&resp.Signatures[0], false)
		if err != nil {
			oc.logger.Error().Msgf("fail to encode the signature")
			return signing.SignatureV2{}, err
		}

		pubkey, err := legacybech32.UnmarshalPubKey(legacybech32.AccPK, signMsg.Pk) //nolint
		if err != nil {
			oc.logger.Error().Err(err).Msgf("fail to get the pubkey")
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

func (oc *OppyChainInstance) doTssSign(msg *tssclient.TssSignigMsg) (keysign.Response, error) {
	resp, err := oc.tssServer.KeySign(msg.Pk, msg.Msgs, msg.BlockHeight, msg.Signers, tssclient.TssVersion)
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to generate the tss signature")
		return keysign.Response{}, err
	}
	return resp, nil
}

// SimBroadcastTx broadcast the tx to the oppyChain to get gas estimation
func (oc *OppyChainInstance) SimBroadcastTx(ctx context.Context, conn grpc1.ClientConn, txbytes []byte) (uint64, error) {
	// Broadcast the tx via gRPC. We create a new client for the Protobuf Tx
	// service.
	txClient := cosTx.NewServiceClient(conn)
	// We then call the BroadcastTx method on this client.
	grpcRes, err := txClient.Simulate(ctx, &cosTx.SimulateRequest{TxBytes: txbytes})
	if err != nil {
		return 0, err
	}
	gasUsed := grpcRes.GetGasInfo().GasUsed
	return gasUsed, nil
}

// GasEstimation this function get the estimation of the fee
func (oc *OppyChainInstance) GasEstimation(conn grpc1.ClientConn, sdkMsg []sdk.Msg, accSeq uint64, tssSignMsg *tssclient.TssSignigMsg) (uint64, error) {
	encoding := MakeEncodingConfig()
	encCfg := encoding
	// Create a new TxBuilder.
	txBuilder := encCfg.TxConfig.NewTxBuilder()

	err := txBuilder.SetMsgs(sdkMsg...)
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to query the gas price")
		return 0, err
	}
	// txBuilder.SetGasLimit(0)

	key, err := oc.Keyring.Key("operator")
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to get the operator key")
		return 0, err
	}
	var pubKey coscrypto.PubKey
	if tssSignMsg == nil {
		pubKey = key.GetPubKey()
	} else {
		pk := tssSignMsg.Pk
		cPk, err := legacybech32.UnmarshalPubKey(legacybech32.AccPK, pk) //nolint
		if err != nil {
			oc.logger.Error().Err(err).Msgf("fail to get the public key from bech32 format")
			return 0, err
		}
		pubKey = cPk
	}

	sigV2 := signing.SignatureV2{
		PubKey: pubKey,
		Data: &signing.SingleSignatureData{
			SignMode:  encCfg.TxConfig.SignModeHandler().DefaultMode(),
			Signature: nil,
		},
		Sequence: accSeq,
	}

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return 0, err
	}

	txBytes, err := oc.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to encode the tx")
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	gasUsed, err := oc.SimBroadcastTx(ctx, conn, txBytes)
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to estimate gas consumption from simulation")
		return 0, err
	}

	gasUsedDec := sdk.NewDecFromIntWithPrec(sdk.NewIntFromUint64(gasUsed), 0)
	gasWanted := gasUsedDec.Mul(sdk.MustNewDecFromStr(config.GASFEERATIO)).RoundInt64()
	return uint64(gasWanted), nil
}

func (oc *OppyChainInstance) prepareTssPool(creator sdk.AccAddress, pubKey, height string) error {
	msg := types.NewMsgCreateCreatePool(creator, pubKey, height)

	dHeight, err := strconv.ParseInt(height, 10, 64)
	if err != nil {
		oc.logger.Error().Err(err).Msgf("fail to parse the height")
		return err
	}

	item := tssPoolMsg{
		msg,
		creator,
		pubKey,
		dHeight,
	}
	oc.poolUpdateLocker.Lock()
	// we store the latest two tss pool outReceiverAddress
	oc.keyGenCache = append(oc.keyGenCache, item)
	oc.poolUpdateLocker.Unlock()
	return nil
}

// GetLastBlockHeightWithLock gets the current block height
func (oc *OppyChainInstance) GetLastBlockHeightWithLock() (int64, error) {
	oc.grpcLock.Lock()
	b, err := GetLastBlockHeight(oc.GrpcClient)
	oc.grpcLock.Unlock()
	if err != nil {
		err2 := oc.RetryOppyChain()
		if err2 != nil {
			oc.logger.Error().Err(err).Msgf("we fail to reset the oppychain")
		}
	}
	return b, err
}

// GetBlockByHeight get the block based on the 'oppyRollbackGap'
func (oc *OppyChainInstance) GetBlockByHeight(conn grpc1.ClientConn, blockHeight int64) (*prototypes.Block, error) {
	block, err := GetBlockByHeight(conn, blockHeight)
	return block, err
}

// CheckAndUpdatePool send the tx to the oppy pub_chain, if the pool outReceiverAddress is updated, it returns true
func (oc *OppyChainInstance) CheckAndUpdatePool(conn grpc1.ClientConn, blockHeight int64) (bool, string) {
	oc.poolUpdateLocker.Lock()
	if len(oc.keyGenCache) < 1 {
		oc.poolUpdateLocker.Unlock()
		// no need to submit
		return true, ""
	}
	el := oc.keyGenCache[0]
	oc.poolUpdateLocker.Unlock()
	if el.blockHeight <= blockHeight {
		oc.logger.Info().Msgf("we are submitting the block at height>>>>>>>>%v\n", el.blockHeight)
		ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
		defer cancel()

		acc, err := queryAccount(conn, el.creator.String(), oc.grpcAddr)
		if err != nil {
			oc.logger.Error().Err(err).Msg("Fail to query the account")
			return false, ""
		}

		gasWanted, err := oc.GasEstimation(conn, []sdk.Msg{el.msg}, acc.GetSequence(), nil)
		if err != nil {
			oc.logger.Error().Err(err).Msg("Fail to get the gas estimation")
			return false, ""
		}
		key, err := oc.Keyring.Key("operator")
		if err != nil {
			oc.logger.Error().Err(err).Msg("fail to get the operator key")
			return false, ""
		}
		txBuilder, err := oc.genSendTx(key, []sdk.Msg{el.msg}, acc.GetSequence(), acc.GetAccountNumber(), gasWanted, nil)
		if err != nil {
			oc.logger.Error().Err(err).Msg("fail to generate the tx")
			return false, ""
		}
		txBytes, err := oc.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
		if err != nil {
			oc.logger.Error().Err(err).Msg("fail to encode the tx")
			return false, ""
		}
		ok, resp, err := oc.BroadcastTx(ctx, conn, txBytes, false)
		if err != nil || !ok {
			oc.logger.Error().Err(err).Msgf("fail to broadcast the tx->%v", resp)
			return false, ""
		}
		// we remove the successful keygen request
		oc.poolUpdateLocker.Lock()
		oc.keyGenCache = oc.keyGenCache[1:]
		oc.poolUpdateLocker.Unlock()
		oc.logger.Info().Msgf("successfully broadcast the pool info")
		return true, el.poolPubKey
	}
	return true, ""
}

// CheckOutBoundTx checks
func (oc *OppyChainInstance) CheckOutBoundTx(conn grpc1.ClientConn, txBlockHeight int64, rawTx tendertypes.Tx) {
	pools := oc.GetPool()
	if pools[0] == nil || pools[1] == nil {
		return
	}
	poolAddress := []sdk.AccAddress{pools[0].OppyAddress, pools[1].OppyAddress}
	encodingConfig := oc.encoding

	tx, err := encodingConfig.TxConfig.TxDecoder()(rawTx)
	if err != nil {
		oc.logger.Info().Msgf("fail to decode the data and skip this tx")
		return
	}

	txWithMemo, ok := tx.(sdk.TxWithMemo)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	memo := txWithMemo.GetMemo()

	var txMemo bcommon.BridgeMemo
	err = json.Unmarshal([]byte(memo), &txMemo)
	if err != nil {
		oc.logger.Error().Err(err).Msgf("fail to parse the memo with %v", memo)
		return
	}
	if !ethcommon.IsHexAddress(txMemo.Dest) {
		oc.logger.Error().Msgf("not a valid erc20 address")
		return
	}
	for _, msg := range txWithMemo.GetMsgs() {
		switch eachMsg := msg.(type) {
		case *banktypes.MsgSend:
			txClient := cosTx.NewServiceClient(conn)
			txquery := cosTx.GetTxRequest{Hash: hex.EncodeToString(rawTx.Hash())}
			t, err := txClient.GetTx(ctx, &txquery)
			if err != nil {
				oc.logger.Error().Err(err).Msgf("fail to query the tx")
				continue
			}

			if t.TxResponse.Code != 0 {
				//		this means this tx is not a successful tx
				zlog.Warn().Msgf("not a valid top up message with error code %v (%v)", t.TxResponse.Code, t.TxResponse.RawLog)
				continue
			}

			err = oc.processMsg(txBlockHeight, poolAddress, pools[1].EthAddress, txMemo, eachMsg, rawTx.Hash())
			if err != nil {
				if err.Error() != "not a top up message to the pool" {
					oc.logger.Error().Err(err).Msgf("fail to process the message, it is not a top up message")
				}
			}

		default:
			continue
		}
	}
}
