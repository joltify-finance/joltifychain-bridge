package oppybridge

import (
	"context"
	"errors"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"strconv"

	"gitlab.com/oppy-finance/oppy-bridge/common"

	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
)

// SubmitOutboundTx submit the outbound record to oppy chain
func (oc *OppyChainInstance) SubmitOutboundTx(operator keyring.Info, requestID string, blockHeight int64, pubchainTx string) error {
	var err error
	if operator == nil {
		operator, err = oc.Keyring.Key("operator")
		if err != nil {
			return err
		}
	}
	acc, err := queryAccount(operator.GetAddress().String(), oc.grpcClient)
	if err != nil {
		oc.logger.Error().Err(err).Msgf("fail to query the account")
		return err
	}
	accSeq, accNum := acc.GetSequence(), acc.GetAccountNumber()

	outboundMsg := vaulttypes.MsgCreateOutboundTx{
		Creator:     operator.GetAddress(),
		RequestID:   requestID,
		OutboundTx:  pubchainTx,
		BlockHeight: strconv.FormatInt(blockHeight, 10),
	}

	ok, _, err := oc.composeAndSend(operator, &outboundMsg, accSeq, accNum, nil, operator.GetAddress())
	if !ok || err != nil {
		oc.logger.Error().Err(err).Msgf("fail to submit the outbound tx record")
		return errors.New("fail to broadcast the outbound tx record")
	}
	return nil
}

// GetPubChainSubmittedTx get the submitted mint tx
func (oc *OppyChainInstance) GetPubChainSubmittedTx(req common.OutBoundReq) (string, error) {
	reqStr := req.Hash().Hex()
	oc.logger.Info().Msgf("we check the hash %v\n", reqStr)
	vaultQuery := vaulttypes.NewQueryClient(oc.grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	outboundTxRequest := vaulttypes.QueryGetOutboundTxRequest{RequestID: reqStr}
	resp, err := vaultQuery.OutboundTx(ctx, &outboundTxRequest)
	if err != nil {
		return "", err
	}

	validators, _ := oc.GetLastValidator()
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
