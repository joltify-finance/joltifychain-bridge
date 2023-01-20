package cosbridge

import (
	"context"
	"encoding/json"
	"strconv"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	grpc1 "github.com/gogo/protobuf/grpc"
	"github.com/joltify-finance/joltify_lending/x/vault/types"
	zlog "github.com/rs/zerolog/log"
	prototypes "github.com/tendermint/tendermint/proto/tendermint/types"
	tendertypes "github.com/tendermint/tendermint/types"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (jc *JoltChainInstance) TerminateBridge() error {
	jc.CosHandler.TerminateCosmosClient()
	jc.CosHandler.TerminateTss()
	return nil
}

func (jc *JoltChainInstance) prepareTssPool(creator sdk.AccAddress, pubKey, height string) error {
	msg := types.NewMsgCreateCreatePool(creator, pubKey, height)

	dHeight, err := strconv.ParseInt(height, 10, 64)
	if err != nil {
		jc.logger.Error().Err(err).Msgf("fail to parse the height")
		return err
	}

	item := tssPoolMsg{
		msg,
		creator,
		pubKey,
		dHeight,
	}
	jc.poolUpdateLocker.Lock()
	// we store the latest two tss pool outReceiverAddress
	jc.keyGenCache = append(jc.keyGenCache, item)
	jc.poolUpdateLocker.Unlock()
	return nil
}

// GetLastBlockHeightWithLock gets the current block height
func (jc *JoltChainInstance) GetLastBlockHeightWithLock() (int64, error) {
	b, err := jc.CosHandler.GetLastBlockHeightWithLock()
	return b, err
}

func (jc *JoltChainInstance) GetCurrentNewBlockChain() <-chan ctypes.ResultEvent {
	return jc.CosHandler.GetCurrentNewBlockChain()
}

func (jc *JoltChainInstance) GetChannelQueueNewBlockChain() chan ctypes.ResultEvent {
	return jc.CosHandler.GetChannelQueueNewBlockChain()
}

func (jc *JoltChainInstance) GetCurrentNewValidator() <-chan ctypes.ResultEvent {
	return jc.CosHandler.GetCurrentNewValidator()
}

func (jc *JoltChainInstance) GetChannelQueueValidator() chan ctypes.ResultEvent {
	return jc.CosHandler.GetChannelQueueNewValidator()
}

// GetBlockByHeight get the block based on the 'oppyRollbackGap'
func (jc *JoltChainInstance) GetBlockByHeight(conn grpc1.ClientConn, blockHeight int64) (*prototypes.Block, error) {
	block, err := bcommon.GetBlockByHeight(conn, blockHeight)
	return block, err
}

func (jc *JoltChainInstance) RetryJoltifyChain() error {
	return jc.CosHandler.RetryJoltifyChain(false)
}

// CheckAndUpdatePool send the tx to the joltify pub_chain, if the pool outReceiverAddress is updated, it returns true
func (jc *JoltChainInstance) CheckAndUpdatePool(conn grpc1.ClientConn, blockHeight int64) (bool, string) {
	jc.poolUpdateLocker.Lock()
	if len(jc.keyGenCache) < 1 {
		jc.poolUpdateLocker.Unlock()
		// no need to submit
		return true, ""
	}
	el := jc.keyGenCache[0]
	jc.poolUpdateLocker.Unlock()
	if el.blockHeight <= blockHeight {
		jc.logger.Info().Msgf("we are submitting the create pool message at height>>>>>>>>%v\n", el.blockHeight)
		ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
		defer cancel()

		acc, err := bcommon.QueryAccount(conn, el.creator.String(), "")
		if err != nil {
			jc.logger.Error().Err(err).Msg("Fail to query the Account")
			return false, ""
		}

		gasWanted, err := jc.CosHandler.GasEstimation(conn, []sdk.Msg{el.msg}, acc.GetSequence(), nil)
		if err != nil {
			jc.logger.Error().Err(err).Msg("Fail to get the gas estimation")
			return false, ""
		}
		key, err := jc.CosHandler.GetKey("operator")
		if err != nil {
			jc.logger.Error().Err(err).Msg("fail to get the operator key")
			return false, ""
		}
		txBuilder, err := jc.CosHandler.GenSendTx(key, []sdk.Msg{el.msg}, acc.GetSequence(), acc.GetAccountNumber(), gasWanted, nil)
		if err != nil {
			jc.logger.Error().Err(err).Msg("fail to generate the tx")
			return false, ""
		}
		txBytes, err := jc.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
		if err != nil {
			jc.logger.Error().Err(err).Msg("fail to encode the tx")
			return false, ""
		}
		ok, resp, err := jc.CosHandler.BroadcastTx(ctx, conn, txBytes, false)
		if err != nil || !ok {
			jc.logger.Error().Err(err).Msgf("fail to broadcast the tx->%v", resp)
			return false, ""
		}
		// we remove the successful keygen request
		jc.poolUpdateLocker.Lock()
		jc.keyGenCache = jc.keyGenCache[1:]
		jc.poolUpdateLocker.Unlock()
		jc.logger.Info().Msgf("successfully broadcast the pool info")
		return true, el.poolPubKey
	}
	return true, ""
}

// CheckOutBoundTx checks
func (jc *JoltChainInstance) CheckOutBoundTx(conn grpc1.ClientConn, txBlockHeight int64, rawTx tendertypes.Tx) {
	pools := jc.GetPool()
	if pools[0] == nil || pools[1] == nil {
		return
	}
	poolAddress := []sdk.AccAddress{pools[0].CosAddress, pools[1].CosAddress}
	encodingConfig := jc.encoding

	tx, err := encodingConfig.TxConfig.TxDecoder()(rawTx)
	if err != nil {
		jc.logger.Debug().Msgf("fail to decode the data and skip this tx")
		return
	}

	txWithMemo, ok := tx.(sdk.TxWithMemo)
	if !ok {
		return
	}

	memo := txWithMemo.GetMemo()

	var txMemo bcommon.BridgeMemo
	err = json.Unmarshal([]byte(memo), &txMemo)
	if err != nil {
		jc.logger.Error().Err(err).Msgf("fail to parse the memo with %v", memo)
		return
	}
	switch txMemo.ChainType {
	case "ATOM":
		_, err := bcommon.AddressStringToBytes("cosmos", txMemo.Dest)
		if err != nil {
			jc.logger.Error().Msgf("not a valid cosmos address")
			return
		}
	default:
		if !ethcommon.IsHexAddress(txMemo.Dest) {
			jc.logger.Error().Msgf("not a valid erc20 address")
			return
		}
	}

	for _, msg := range txWithMemo.GetMsgs() {
		switch eachMsg := msg.(type) {
		case *banktypes.MsgSend:

			t, err := bcommon.GetGivenTx(conn, rawTx.Hash())
			if err != nil {
				jc.logger.Error().Err(err).Msgf("fail to query the tx")
				continue
			}

			if t.TxResponse.Code != 0 {
				//		this means this tx is not a successful tx
				zlog.Warn().Msgf("not a valid top up message with error code %v (%v)", t.TxResponse.Code, t.TxResponse.RawLog)
				continue
			}

			err = jc.processMsg(txBlockHeight, poolAddress, pools[1], txMemo, eachMsg, rawTx.Hash())
			if err != nil {
				if err.Error() != "not a top up message to the pool" {
					jc.logger.Error().Err(err).Msgf("fail to process the message, it is not a top up message")
				}
			}

		default:
			continue
		}
	}
}

func (jc *JoltChainInstance) SetKey(uid string, data, pass []byte) error {
	return jc.CosHandler.SetKey(uid, data, pass)
}

func (jc *JoltChainInstance) GetTssNodeID() string {
	return jc.CosHandler.GetTssNodeID()
}
