package cosbridge

import (
	"html"

	"google.golang.org/grpc"

	sdk "github.com/cosmos/cosmos-sdk/types"
	grpc1 "github.com/gogo/protobuf/grpc"
	"gitlab.com/joltify/joltifychain-bridge/common"

	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
)

func prepareIssueTokenRequest(item *common.InBoundReq, creatorAddr, index string) (*vaulttypes.MsgCreateIssueToken, error) {
	userAddr, coin, _ := item.GetInboundReqInfo()
	a, err := vaulttypes.NewMsgCreateIssueToken(creatorAddr, index, coin.String(), userAddr.String())
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (jc *JoltChainInstance) CheckWhetherAlreadyExist(client *grpc.ClientConn, index string) bool {
	return jc.CosHandler.CheckWhetherIssueTokenAlreadyExist(client, index)
}

func (jc *JoltChainInstance) CheckIssueTokenTxStatus(client *grpc.ClientConn, index string, retryNum uint64) error {
	return jc.CosHandler.CheckIssueTokenTxStatus(client, index, retryNum)
}

// DoProcessInBound mint the token in joltify chain
func (jc *JoltChainInstance) DoProcessInBound(conn grpc1.ClientConn, items []*common.InBoundReq) (map[string]string, error) {
	signMsgs := make([]*tssclient.TssSignigMsg, len(items))
	issueReqs := make([]sdk.Msg, len(items))

	blockHeight, err := jc.GetLastBlockHeightWithLock()
	if err != nil {
		jc.logger.Error().Err(err).Msgf("fail to get the block height in process the inbound tx")
		return nil, err
	}
	roundBlockHeight := blockHeight / ROUNDBLOCK
	for i, item := range items {
		index := item.Hash().Hex()
		_, _, poolAddress, poolPk := item.GetAccountInfo()
		jc.logger.Info().Msgf("we are about to prepare the tx with other nodes with index %v", index)
		issueReq, err := prepareIssueTokenRequest(item, poolAddress.String(), index)
		if err != nil {
			jc.logger.Error().Err(err).Msg("fail to prepare the issuing of the token")
			continue
		}

		tick := html.UnescapeString("&#" + "128296" + ";")
		jc.logger.Info().Msgf("%v we do the top up for %v at height %v", tick, issueReq.Receiver.String(), roundBlockHeight)
		signMsg := tssclient.TssSignigMsg{
			Pk:          poolPk,
			Signers:     nil,
			BlockHeight: roundBlockHeight,
			Version:     tssclient.TssVersion,
		}
		signMsgs[i] = &signMsg
		issueReqs[i] = issueReq
	}
	// as in a group, the accseq MUST has been sorted.
	accSeq, accNum, poolAddress, _ := items[0].GetAccountInfo()
	// for batchsigning, the signMsgs for all the members in the group is the same
	txHashes, err := jc.CosHandler.BatchComposeAndSend(conn, issueReqs, accSeq, accNum, signMsgs[0], poolAddress.String())
	if err != nil {
		jc.logger.Error().Msgf("we fail to process one or more txs")
	}

	hashIndexMap := make(map[string]string)
	for _, el := range items {
		index := el.Hash().Hex()
		txHash := txHashes[el.AccSeq]
		hashIndexMap[index] = txHash
	}

	return hashIndexMap, nil
}
