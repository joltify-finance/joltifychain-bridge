package cosbridge

import (
	"context"
	"errors"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	grpc1 "github.com/gogo/protobuf/grpc"

	"gitlab.com/joltify/joltifychain-bridge/common"

	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
)

// SubmitOutboundTx submit the outbound record to joltify chain
func (jc *JoltChainInstance) SubmitOutboundTx(conn grpc1.ClientConn, operator keyring.Info, requestID string, blockHeight int64, pubchainTx string, fee sdk.Coins, chainType, inTxHash string, receiverAddress sdk.AccAddress, needMint bool) error {
	var err error
	if operator == nil {
		operator, err = jc.Keyring.Key("operator")
		if err != nil {
			return err
		}
	}
	acc, err := queryAccount(conn, operator.GetAddress().String(), "")
	if err != nil {
		jc.logger.Error().Err(err).Msgf("fail to query the Account")
		return err
	}
	accSeq, accNum := acc.GetSequence(), acc.GetAccountNumber()

	outboundMsg := vaulttypes.MsgCreateOutboundTx{
		Creator:         operator.GetAddress(),
		RequestID:       requestID,
		OutboundTx:      pubchainTx,
		BlockHeight:     strconv.FormatInt(blockHeight, 10),
		Feecoin:         fee,
		ChainType:       chainType,
		NeedMint:        needMint,
		InTxHash:        inTxHash,
		ReceiverAddress: receiverAddress,
	}

	ok, _, err := jc.composeAndSend(conn, operator, &outboundMsg, accSeq, accNum, nil, operator.GetAddress())
	if !ok || err != nil {
		jc.logger.Error().Err(err).Msgf("fail to submit the outbound tx record")
		return errors.New("fail to broadcast the outbound tx record")
	}
	return nil
}

// GetPubChainSubmittedTx get the submitted mint tx
func (jc *JoltChainInstance) GetPubChainSubmittedTx(req common.OutBoundReq) (string, error) {
	reqStr := req.Hash().Hex()
	jc.logger.Info().Msgf("we check the hash %v\n", reqStr)
	vaultQuery := vaulttypes.NewQueryClient(jc.GrpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	outboundTxRequest := vaulttypes.QueryGetOutboundTxRequest{RequestID: reqStr}
	resp, err := vaultQuery.OutboundTx(ctx, &outboundTxRequest)
	if err != nil {
		return "", err
	}

	validators, _ := jc.GetLastValidator()
	min := float32(len(validators)*2) / float32(3)

	target := ""
	for key, value := range resp.OutboundTx.Items {
		if len(value.Entry) >= int(min) {
			target = key
			break
		}
	}
	return target, nil
}
