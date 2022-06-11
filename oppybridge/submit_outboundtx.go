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
func (jc *OppyChainInstance) SubmitOutboundTx(operator keyring.Info, requestID string, blockHeight int64, pubchainTx string) error {
	var err error
	if operator == nil {
		operator, err = jc.Keyring.Key("operator")
		if err != nil {
			return err
		}
	}
	acc, err := queryAccount(operator.GetAddress().String(), jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msgf("fail to query the account")
		return err
	}
	accSeq, accNum := acc.GetSequence(), acc.GetAccountNumber()

	outboundMsg := vaulttypes.MsgCreateOutboundTx{
		Creator:     operator.GetAddress(),
		RequestID:   requestID,
		OutboundTx:  pubchainTx,
		BlockHeight: strconv.FormatInt(blockHeight, 10),
	}

	ok, _, err := jc.composeAndSend(operator, &outboundMsg, accSeq, accNum, nil, operator.GetAddress())
	if !ok || err != nil {
		jc.logger.Error().Err(err).Msgf("fail to submit the outbound tx record")
		return errors.New("fail to broadcast the outbound tx record")
	}
	return nil
}

// GetPubChainSubmittedTx get the submitted mint tx
func (jc *OppyChainInstance) GetPubChainSubmittedTx(req common.OutBoundReq) (string, error) {
	reqStr := req.Hash().Hex()
	jc.logger.Info().Msgf("we check the hash %v\n", reqStr)
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
