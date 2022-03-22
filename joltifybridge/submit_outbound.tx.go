package joltifybridge

import (
	"context"
	"errors"
	"strconv"

	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

// SubmitOutboundTx submit the outbound record to joltify chain
func (jc *JoltifyChainInstance) SubmitOutboundTx(req OutBoundReq, pubchainTx string) error {
	operator, err := jc.Keyring.Key("operator")
	if err != nil {
		return err
	}

	acc, err := QueryAccount(operator.GetAddress().String(), jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msgf("fail to query the account")
		return err
	}
	accSeq, accNum := acc.GetSequence(), acc.GetAccountNumber()

	outboundMsg := vaulttypes.MsgCreateOutboundTx{
		Creator:     operator.GetAddress(),
		RequestID:   req.Hash().String(),
		OutboundTx:  pubchainTx,
		BlockHeight: strconv.FormatInt(req.blockHeight, 10),
	}

	ok, _, err := jc.composeAndSend(&outboundMsg, accSeq, accNum, nil)
	if !ok || err != nil {
		jc.logger.Error().Err(err).Msgf("fail to broadcast the outbound tx record")
		return errors.New("fail to broadcast the outbound tx record")
	}
	return nil
}

func (jc *JoltifyChainInstance) GetPubChainSubmittedTx(req OutBoundReq) (string, error) {
	reqStr := req.Hash().Hex()
	vaultQuery := vaulttypes.NewQueryClient(jc.grpcClient)
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
		if len(value.Address) >= int(min) {
			target = key
			break
		}
	}
	return target, nil
}
