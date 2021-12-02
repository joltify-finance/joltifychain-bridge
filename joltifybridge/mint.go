package joltifybridge

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/pubchain"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/tssclient"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

func prepareIssueTokenRequest(item *pubchain.AccountInboundReq, creatorAddr, index string) (*vaulttypes.MsgCreateIssueToken, error) {
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

// MintCoin mint the token in joltify chain
func (jc *JoltifyChainBridge) MintCoin(item *pubchain.AccountInboundReq) error {
	poolInfo, err := jc.QueryLastPoolAddress()
	if err != nil {
		jc.logger.Error().Err(err).Msgf("error in get pool with error %v", err)
		return err

	}
	if len(poolInfo) != 2 {
		jc.logger.Info().Msgf("fail to query the pool with length %v", len(poolInfo))
		return errors.New("not enough signer")
	}
	creatorAddr := poolInfo[1].CreatePool.PoolAddr
	fmt.Printf("------------we sign with %v\n", poolInfo[1].CreatePool.PoolPubKey)

	acc, err := queryAccount(creatorAddr.String(), jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to query the pool account")
		return err
	}

	index := strconv.FormatUint(acc.GetSequence(), 10)
	issueReq, err := prepareIssueTokenRequest(item, creatorAddr.String(), index)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to prepare the issuing of the token")
	}

	_, _, _, height := item.GetInboundReqInfo()
	signMsg := tssclient.TssSignigMsg{
		Pk:          poolInfo[1].CreatePool.PoolPubKey,
		Signers:     nil,
		BlockHeight: height,
		Version:     tssclient.TssVersion,
	}

	jc.logger.Info().Msgf("we do the broadcast top-up token message!!\n")
	fmt.Printf(">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>\n")
	fmt.Printf("%v\n", issueReq.Coin)
	fmt.Printf("The Vault address: %v\n", issueReq.Creator.String())
	fmt.Printf("The User Wallet Address: %v\n", issueReq.Receiver.String())
	fmt.Printf(">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>\n")

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
	fmt.Printf("we have successfully broadcast the signature")

	return nil
}
