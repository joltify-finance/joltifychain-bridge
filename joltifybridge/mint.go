package joltifybridge

import (
	"errors"
	"sync"

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

	// we need to check whether the tx already exist
	index := item.Hash().Hex()
	if jc.CheckWhetherAlreadyExist(index) {
		jc.logger.Warn().Msg("#################already submitted by others")
		return nil
	}

	jc.logger.Info().Msgf("we are about to prepare the tx with other nodes with index %v at height %v", index, item.GetItemHeight())
	issueReq, err := prepareIssueTokenRequest(item, joltCreatorAddr.String(), index)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to prepare the issuing of the token")
		return err
	}

	_, _, _, height := item.GetInboundReqInfo()
	signMsg := tssclient.TssSignigMsg{
		Pk:          pool[1].Pk,
		Signers:     nil,
		BlockHeight: int64(height),
		Version:     tssclient.TssVersion,
	}

	// we always increase the account seq regardless the tx successful or not
	accNum, accSeq := jc.AcquirePoolAccountInfo()
	//acc, err := queryAccount(pool[1].JoltifyAddress.String(), jc.grpcClient)
	//if err != nil {
	//	return errors.New("fail to query the account ")
	//}
	//accNum, accSeq := acc.GetAccountNumber(), acc.GetSequence()

	jc.logger.Info().Msgf("we do the top up for %v at height: %v with seq:%v", issueReq.Receiver.String(), height, accSeq)
	txByte, err := jc.composeAndSend(issueReq, accSeq, accNum, &signMsg, pool[1].JoltifyAddress.String())
	if err != nil {
		jc.logger.Error().Err(err).Msgf("fail to compose the send tx")
		return errors.New("fail to process the inbound tx")
	}
	bItem := broadcast{
		item,
		txByte,
	}
	data, exist := jc.broadcastChannel.Load(accNum)
	if exist == false {
		storage := sync.Map{}
		storage.Store(accSeq, &bItem)
		jc.broadcastChannel.Store(accNum, &storage)
	} else {
		storage := data.(*sync.Map)
		storage.Store(accSeq, &bItem)
		jc.broadcastChannel.Store(accNum, storage)
	}
	jc.IncreaseAccountSeq()
	return nil
}
