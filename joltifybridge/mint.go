package joltifybridge

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/pubchain"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/tssclient"
	vaulttypes "gitlab.com/joltify/joltifychain/joltifychain/x/vault/types"
)

func prepareIssueTokenRequest(item *pubchain.AccountInboundReq, creatorPub string) (*vaulttypes.MsgCreateIssueToken, error) {
	userAcc, _, coin, _ := item.GetInboundReqInfo()

	receiver, err := misc.EthAddressToJoltAddr(userAcc)
	if err != nil {
		return nil, err
	}

	pk, err := sdk.GetPubKeyFromBech32(sdk.Bech32PubKeyTypeAccPub, creatorPub)
	if err != nil {
		return nil, err
	}
	fmt.Printf("pubkey is %v\n", creatorPub)

	creator, err := sdk.AccAddressFromHex(pk.Address().String())
	if err != nil {
		return nil, err
	}

	fmt.Printf("address is %v\n", creator.String())

	a, err := vaulttypes.NewMsgCreateIssueToken(creator.String(), userAcc.String(), coin.String(), receiver.String())
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
	creatorPub := poolInfo[1].CreatePool.PoolPubKey
	fmt.Printf("------------we sign with %v\n", poolInfo[1].CreatePool.PoolPubKey)

	issueReq, err := prepareIssueTokenRequest(item, creatorPub)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to prepare the issuing of the token")
	}

	var accSeq, accNum uint64
	acc, err := queryAccount(issueReq.Creator.String(), jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to query the account")
		accSeq = 1
		accNum = 1
	} else {
		accSeq = acc.GetSequence()
		accNum = acc.GetAccountNumber()
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

	txbytes, _, err := jc.genSendTx([]sdk.Msg{issueReq}, accSeq, accNum, &signMsg)
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
	return nil
}
