package cosbridge

import (
	"context"
	"errors"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	grpc1 "github.com/gogo/protobuf/grpc"

	bcommon "gitlab.com/joltify/joltifychain-bridge/common"

	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
)

// SubmitOutboundTx submit the outbound record to joltify chain
func (jc *JoltChainInstance) SubmitOutboundTx(conn grpc1.ClientConn, operator keyring.Info, requestID string, blockHeight int64, pubchainTx string, fee sdk.Coins, chainType, inTxHash string, receiverAddress sdk.AccAddress, needMint bool) error {
	var err error
	if operator == nil {
		operator, err = jc.CosHandler.GetKey("operator")
		if err != nil {
			return err
		}
	}
	acc, err := bcommon.QueryAccount(conn, operator.GetAddress().String(), "")
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

	ok, _, err := jc.CosHandler.ComposeAndSend(conn, operator, &outboundMsg, accSeq, accNum, nil, operator.GetAddress().String())
	if !ok || err != nil {
		jc.logger.Error().Err(err).Msgf("fail to submit the outbound tx record")
		return errors.New("fail to broadcast the outbound tx record")
	}
	return nil
}

// GetPubChainSubmittedTx get the submitted mint tx
func (jc *JoltChainInstance) GetPubChainSubmittedTx(req bcommon.OutBoundReq, validatorNum int) (string, error) {
	reqStr := req.Hash().Hex()
	jc.logger.Info().Msgf("we check the hash %v\n", reqStr)

	vaultQuery := vaulttypes.NewQueryClient(jc.CosHandler.GrpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	outboundTxRequest := vaulttypes.QueryGetOutboundTxRequest{RequestID: reqStr}
	resp, err := vaultQuery.OutboundTx(ctx, &outboundTxRequest)
	if err != nil {
		return "", err
	}

	min := float32(validatorNum*2) / float32(3)

	target := ""
	for key, value := range resp.OutboundTx.Items {
		if len(value.Entry) >= int(min) {
			target = key
			break
		}
	}
	return target, nil
}
