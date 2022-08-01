package oppybridge

import (
	"html"

	sdk "github.com/cosmos/cosmos-sdk/types"
	grpc1 "github.com/gogo/protobuf/grpc"
	"gitlab.com/oppy-finance/oppy-bridge/common"

	"gitlab.com/oppy-finance/oppy-bridge/tssclient"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
)

func prepareIssueTokenRequest(item *common.InBoundReq, creatorAddr, index string) (*vaulttypes.MsgCreateIssueToken, error) {
	userAddr, _, coin, _ := item.GetInboundReqInfo()

	a, err := vaulttypes.NewMsgCreateIssueToken(creatorAddr, index, coin.String(), userAddr.String())
	if err != nil {
		return nil, err
	}
	return a, nil
}

// ProcessInBound mint the token in oppy chain
func (oc *OppyChainInstance) ProcessInBound(conn grpc1.ClientConn, items []*common.InBoundReq) (map[string]string, error) {

	var needToBeProcessed []*common.InBoundReq
	for _, item := range items {
		// we need to check against the previous account sequence
		index := item.Hash().Hex()
		if oc.CheckWhetherAlreadyExist(conn, index) {
			oc.logger.Warn().Msg("already submitted by others")
			continue
		}
		needToBeProcessed = append(needToBeProcessed, item)
	}

	var signMsgs []*tssclient.TssSignigMsg
	var issueReqs []sdk.Msg

	blockHeight, err := oc.GetLastBlockHeightWithLock()
	if err != nil {
		oc.logger.Error().Err(err).Msgf("fail to get the block height in process the inbound tx")
		return nil, err
	}
	roundBlockHeight := blockHeight / ROUNDBLOCK
	for _, item := range needToBeProcessed {
		index := item.Hash().Hex()
		_, _, poolAddress, poolPk := item.GetAccountInfo()
		oc.logger.Info().Msgf("we are about to prepare the tx with other nodes with index %v", index)
		issueReq, err := prepareIssueTokenRequest(item, poolAddress.String(), index)
		if err != nil {
			oc.logger.Error().Err(err).Msg("fail to prepare the issuing of the token")
			return nil, err
		}

		tick := html.UnescapeString("&#" + "128296" + ";")
		oc.logger.Info().Msgf("%v we do the top up for %v at height %v", tick, issueReq.Receiver.String(), roundBlockHeight)
		signMsg := tssclient.TssSignigMsg{
			Pk:          poolPk,
			Signers:     nil,
			BlockHeight: roundBlockHeight,
			Version:     tssclient.TssVersion,
		}
		signMsgs = append(signMsgs, &signMsg)
		issueReqs = append(issueReqs, issueReq)
	}

	// as in a group, the accseq MUST has been sorted.
	accSeq, accNum, poolAddress, _ := items[0].GetAccountInfo()
	// for batchsigning, the signMsgs for all the members in the grop is the same
	txHashes, err := oc.batchComposeAndSend(conn, issueReqs, accSeq, accNum, signMsgs[0], poolAddress)
	if err != nil {
		oc.logger.Error().Msgf("we fail to process one or more txs")
	}

	hashIndexMap := make(map[string]string)
	for _, el := range items {
		index := el.Hash().Hex()
		txHash := txHashes[el.AccSeq]
		hashIndexMap[index] = txHash
	}

	return hashIndexMap, nil
}
