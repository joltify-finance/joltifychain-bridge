package joltifybridge

import (
	"errors"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain-bridge/pubchain"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

func prepareIssueTokenRequest(item *pubchain.InboundReq, creatorAddr, index string) (*vaulttypes.MsgCreateIssueToken, error) {
	userAddr, _, coin, _ := item.GetInboundReqInfo()

	a, err := vaulttypes.NewMsgCreateIssueToken(creatorAddr, index, coin.String(), userAddr.String())
	if err != nil {
		return nil, err
	}
	return a, nil
}

// ProcessInBound mint the token in joltify chain
func (jc *JoltifyChainInstance) ProcessInBound(item *pubchain.InboundReq) (string, string, error) {
	pool := jc.GetPool()
	if pool[0] == nil {
		jc.logger.Info().Msgf("fail to query the pool with length 1")
		return "", "", errors.New("not enough signer")
	}
	// we need to get the address from the pubkey rather than the eth address
	joltCreatorAddr, err := misc.PoolPubKeyToJoltAddress(pool[1].Pk)
	if err != nil {
		jc.logger.Info().Msgf("fail to convert the eth address to jolt address")
		return "", "", errors.New("invalid address")
	}

	// we always increase the account seq regardless the tx successful or not
	acc, err := queryAccount(pool[1].JoltifyAddress.String(), jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msgf("fail to query the account")
		return "", "", errors.New("invalid account query")
	}
	accSeq, accNum := acc.GetSequence(), acc.GetAccountNumber()
	// we need to check against the previous account sequence
	index := item.Hash().Hex()
	if jc.CheckWhetherAlreadyExist(index) {
		jc.logger.Warn().Msg("already submitted by others")
		return "", "", nil
	}

	jc.logger.Info().Msgf("we are about to prepare the tx with other nodes with index %v", index)
	issueReq, err := prepareIssueTokenRequest(item, joltCreatorAddr.String(), index)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to prepare the issuing of the token")
		return "", "", err
	}

	_, _, _, height := item.GetInboundReqInfo()
	jc.logger.Info().Msgf("we do the top up for %v at height %v", issueReq.Receiver.String(), height)
	signMsg := tssclient.TssSignigMsg{
		Pk:          pool[1].Pk,
		Signers:     nil,
		BlockHeight: height,
		Version:     tssclient.TssVersion,
	}

	ok, txHash, err := jc.composeAndSend(issueReq, accSeq, accNum, &signMsg)
	if err != nil || !ok {
		jc.logger.Error().Err(err).Msgf("fail to broadcast the tx->%v", txHash)
		return "", "", errors.New("fail to process the inbound tx")
	}
	return txHash, index, nil
}
