package joltifybridge

import (
	"context"
	"errors"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain-bridge/pubchain"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

func prepareIssueTokenRequest(item *pubchain.InboundReq, creatorAddr, index string) (*vaulttypes.MsgCreateIssueToken, error) {
	userAcc, _, coin, _ := item.GetInboundReqInfo()

	receiver, err := misc.EthAddressToJoltAddr(userAcc)
	if err != nil {
		return nil, err
	}

	a, err := vaulttypes.NewMsgCreateIssueToken(creatorAddr, index, coin.String(), receiver.String())
	if err != nil {
		return nil, err
	}
	return a, nil
}

// ProcessInBound mint the token in joltify chain
func (jc *JoltifyChainBridge) ProcessInBound(item *pubchain.InboundReq) error {
	//poolInfo, err := jc.QueryLastPoolAddress()
	//if err != nil {
	//	jc.logger.Error().Err(err).Msgf("error in get pool with error %v", err)
	//	return err
	//}
	//if len(poolInfo) != 2 {
	//	jc.logger.Info().Msgf("fail to query the pool with length %v", len(poolInfo))
	//	return errors.New("not enough signer")
	//}
	pool := jc.GetPool()
	if pool[0] == nil {
		jc.logger.Info().Msgf("fail to query the pool with length 1")
		return errors.New("not enough signer")
	}
	creatorAddr := pool[1].address
	joltCreatorAddr, err := misc.EthAddressToJoltAddr(creatorAddr)
	if err != nil {
		jc.logger.Info().Msgf("fail to convert the eth address to jolt address")
		return errors.New("invalid address")
	}

	acc, err := queryAccount(joltCreatorAddr.String(), jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to query the pool account")
		return err
	}

	// we need to check against the previous account sequence
	preIndex := strconv.FormatUint(acc.GetSequence(), 10) + creatorAddr.String()
	if jc.CheckWhetherAlreadyExist(preIndex) {
		jc.logger.Warn().Msg("already submitted by others")
		return nil
	}

	index := strconv.FormatUint(acc.GetSequence(), 10) + creatorAddr.String()
	jc.logger.Info().Msgf("we are about the prepare the tx with other nodes with index %v", index)
	issueReq, err := prepareIssueTokenRequest(item, creatorAddr.String(), index)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to prepare the issuing of the token")
	}

	_, _, _, height := item.GetInboundReqInfo()
	jc.logger.Info().Msgf("we do the top up for %v at height %v", issueReq.Receiver.String(), height)
	signMsg := tssclient.TssSignigMsg{
		Pk:          pool[1].pk,
		Signers:     nil,
		BlockHeight: height,
		Version:     tssclient.TssVersion,
	}

	txbytes, _, err := jc.genSendTx([]sdk.Msg{issueReq}, acc.GetSequence(), acc.GetAccountNumber(), &signMsg)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to generate the tx")
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	ok, resp, err := jc.BroadcastTx(ctx, txbytes)
	if err != nil || !ok {
		jc.logger.Error().Err(err).Msgf("fail to broadcast the tx->%v", resp)
		return err
	}
	jc.logger.Info().Msgf("we have successfully top up %s with %v", issueReq.Receiver.String(), issueReq.Coin.String())

	return nil
}
