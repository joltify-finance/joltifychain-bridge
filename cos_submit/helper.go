package cossubmit

import (
	"context"

	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	coscrypto "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32"
	cosTx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	grpc1 "github.com/gogo/protobuf/grpc"
	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	"github.com/joltify-finance/tss/keygen"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
)

// SimBroadcastTx broadcast the tx to the oppyChain to get gas estimation
func (cs *CosHandler) SimBroadcastTx(ctx context.Context, conn grpc1.ClientConn, txbytes []byte) (uint64, error) {
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
func (cs *CosHandler) GasEstimation(
	conn grpc1.ClientConn,
	sdkMsg []sdk.Msg,
	accSeq uint64,
	tssSignMsg *tssclient.TssSignigMsg,
) (uint64, error) {
	encoding := bcommon.MakeEncodingConfig()
	encCfg := encoding
	// Create a new TxBuilder.
	txBuilder := encCfg.TxConfig.NewTxBuilder()
	err := txBuilder.SetMsgs(sdkMsg...)
	if err != nil {
		cs.logger.Error().Err(err).Msg("fail to query the gas price")
		return 0, err
	}
	// txBuilder.SetGasLimit(0)

	key, err := cs.Keyring.Key("operator")
	if err != nil {
		cs.logger.Error().Err(err).Msg("fail to get the operator key")
		return 0, err
	}
	var pubKey coscrypto.PubKey
	if tssSignMsg == nil {
		pubKey = key.GetPubKey()
	} else {
		pk := tssSignMsg.Pk
		cPk, err := legacybech32.UnmarshalPubKey(legacybech32.AccPK, pk) //nolint
		if err != nil {
			cs.logger.Error().Err(err).Msgf("fail to get the public key from bech32 format")
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

	txBytes, err := cs.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		cs.logger.Error().Err(err).Msg("fail to encode the tx")
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	gasUsed, err := cs.SimBroadcastTx(ctx, conn, txBytes)
	if err != nil {
		cs.logger.Error().Err(err).Msg("fail to estimate gas consumption from simulation")
		return 0, err
	}

	gasUsedDec := sdk.NewDecFromIntWithPrec(sdk.NewIntFromUint64(gasUsed), 0)
	gasWanted := gasUsedDec.Mul(sdk.MustNewDecFromStr(config.GASFEERATIO)).RoundInt64()
	return uint64(gasWanted), nil
}

// queryLastValidatorSet get the last two validator sets
func queryGivenTokenIssueTx(grpcClient grpc1.ClientConn, index string) (*vaulttypes.IssueToken, error) {
	ts := vaulttypes.NewQueryClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	req := vaulttypes.QueryGetIssueTokenRequest{
		Index: index,
	}
	resp, err := ts.IssueToken(ctx, &req)
	if err != nil {
		return nil, err
	}

	return resp.IssueToken, nil
}

// CheckWhetherIssueTokenAlreadyExist check whether it is already existed
func (cs *CosHandler) CheckWhetherIssueTokenAlreadyExist(conn grpc1.ClientConn, index string) bool {
	ret, err := queryGivenTokenIssueTx(conn, index)
	if err != nil {
		return false
	}
	if ret != nil {
		return true
	}
	return false
}

func (cs *CosHandler) GetTssNodeID() string {
	return cs.tssServer.GetTssNodeID()
}

func (cs *CosHandler) KeyGen(pubKeys []string, blockHeight int64, tssVersion string) (keygen.Response, error) {
	resp, err := cs.tssServer.KeyGen(pubKeys, blockHeight, tssclient.TssVersion)
	return resp, err
}

func (cs *CosHandler) SetKey(uid string, data []byte, pass []byte) error {
	err := cs.Keyring.ImportPrivKey(uid, string(data), string(pass))
	return err
}

func (cs *CosHandler) GetKey(uid string) (keyring.Info, error) {
	info, err := cs.Keyring.Key(uid)
	return info, err
}

func (cs *CosHandler) DeleteKey(uid string) error {
	return cs.Keyring.Delete(uid)
}

func (cs *CosHandler) NewMnemonic(uid string) (keyring.Info, error) {
	key, _, err := cs.Keyring.NewMnemonic(uid, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	return key, err
}
