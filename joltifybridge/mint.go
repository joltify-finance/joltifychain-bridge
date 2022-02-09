package joltifybridge

import (
	"context"
	"errors"
	"html"

	sdk "github.com/cosmos/cosmos-sdk/types"
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
func (jc *JoltifyChainInstance) ProcessInBound(item *pubchain.InboundReq) error {
	var txBytes []byte
	pool := jc.GetPool()
	if pool[0] == nil {
		jc.logger.Info().Msgf("fail to query the pool with length 1")
		return errors.New("not enough signer")
	}
	// we need to get the address from the pubkey rather than the eth address
	joltCreatorAddr, err := misc.PoolPubKeyToJoltAddress(pool[1].Pk)
	if err != nil {
		jc.logger.Info().Msgf("fail to convert the eth address to jolt address")
		return errors.New("invalid address")
	}

	// we always increase the account seq regardless the tx successful or not
	accNum, accSeq := jc.AcquirePoolAccountInfo()
	// we need to check against the previous account sequence
	index := item.Hash().Hex()
	if jc.CheckWhetherAlreadyExist(index) {
		jc.logger.Warn().Msg("already submitted by others")
		return nil
	}

	jc.logger.Info().Msgf("we are about the prepare the tx with other nodes with index %v", index)
	issueReq, err := prepareIssueTokenRequest(item, joltCreatorAddr.String(), index)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to prepare the issuing of the token")
		return err
	}

	_, _, _, height := item.GetInboundReqInfo()
	jc.logger.Info().Msgf("we do the top up for %v at height %v", issueReq.Receiver.String(), height)
	signMsg := tssclient.TssSignigMsg{
		Pk:          pool[1].Pk,
		Signers:     nil,
		BlockHeight: height,
		Version:     tssclient.TssVersion,
	}

	gasWanted, err := jc.GasEstimation([]sdk.Msg{issueReq}, accSeq, &signMsg)
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to get the gas estimation")
		return err
	}
	txBuilder, err := jc.genSendTx([]sdk.Msg{issueReq}, accSeq, accNum, gasWanted, &signMsg)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to generate the tx")
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	txBytes, err = jc.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to encode the tx")
		return err
	}

	ok, resp, err := jc.BroadcastTx(ctx, txBytes)
	if err != nil || !ok {
		jc.logger.Error().Err(err).Msgf("fail to broadcast the tx->%v", resp)
		return errors.New("fail to process the inbound tx")
	}
	tick := html.UnescapeString("&#" + "9989" + ";")
	jc.logger.Info().Msgf("%v txid(%v) have successfully top up %s with %v", tick, resp, issueReq.Receiver.String(), issueReq.Coin.String())
	return nil
}
