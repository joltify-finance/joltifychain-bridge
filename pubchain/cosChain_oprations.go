package pubchain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	cosTx "github.com/cosmos/cosmos-sdk/types/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	zlog "github.com/rs/zerolog/log"
	types2 "github.com/tendermint/tendermint/types"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"google.golang.org/grpc"
)

const grpcTimeout = time.Second * 5

func (c *CosMosChainInfo) Terminate() error {
	err := c.CosHandler.TerminateCosmosClient()
	if err != nil {
		c.logger.Error().Err(err).Msgf("fail to terminate the grpc client")
	}
	return nil
}

// AddSubscribe add the subscription to the chain
func (c *CosMosChainInfo) AddSubscribe(ctx context.Context) error {
	return c.CosHandler.AddSubscribe(ctx)
}

func (c *CosMosChainInfo) RetryCosmosChain() error {
	return c.CosHandler.RetryJoltifyChain(false)
}

// processMsg handle the oppychain transactions
func (c *CosMosChainInfo) processMsg(txBlockHeight int64, poolAddress []sdk.AccAddress, memo common.BridgeMemo, msg *banktypes.MsgSend, txHash []byte) (*common.InBoundReq, error) {
	if msg.Amount.IsZero() {
		return nil, errors.New("zero amount")
	}
	txID := strings.ToLower(hex.EncodeToString(txHash))

	receiver, err := sdk.AccAddressFromBech32(memo.Dest)
	if err != nil {
		c.logger.Error().Err(err).Msg("fail to parse the joltify receiver")
		return nil, errors.New("invalid destination")
	}

	pool1Atom := sdk.MustBech32ifyAddressBytes("cosmos", poolAddress[0])
	pool2Atom := sdk.MustBech32ifyAddressBytes("cosmos", poolAddress[1])
	if !((msg.ToAddress == pool1Atom) || msg.ToAddress == pool2Atom) {
		c.logger.Warn().Msg("not a top up message to the pool")
		return nil, errors.New("not a top up message to the pool")
	}

	if len(msg.Amount) != 1 || msg.Amount[0].Denom != "uatom" {
		c.logger.Warn().Msg("not a top up message to the pool")
		return nil, errors.New("we only accept one token and only uatom")
	}

	receivedToken := msg.Amount[0]
	tx := InboundTx{
		txID,
		receiver,
		uint64(txBlockHeight),
		receivedToken,
	}

	txIDBytes, err := hex.DecodeString(txID)
	if err != nil {
		c.logger.Warn().Msgf("invalid tx ID %v\n", txIDBytes)
		return nil, errors.New("invalid tx")
	}

	item := common.NewAccountInboundReq(tx.ReceiverAddress, tx.Token, txIDBytes, txBlockHeight)

	return &item, nil
}

func (c *CosMosChainInfo) processEachCosmosTx(rawTx types2.Tx, pools []*common.PoolInfo, txBlockHeight int64) ([]*common.InBoundReq, error) {
	tx, err := c.encoding.TxConfig.TxDecoder()(rawTx)
	if err != nil {
		c.logger.Debug().Msgf("fail to decode the data and skip this tx")
		return nil, err
	}

	txWithMemo, ok := tx.(sdk.TxWithMemo)
	if !ok {
		return nil, errors.New("invalid tx memo")
	}
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	memo := txWithMemo.GetMemo()

	var txMemo common.BridgeMemo
	err = json.Unmarshal([]byte(memo), &txMemo)
	if err != nil {
		c.logger.Error().Err(err).Msgf("fail to parse the memo with %v", memo)
		return nil, err
	}
	_, err = sdk.AccAddressFromBech32(txMemo.Dest)
	if err != nil || txMemo.ChainType != "JOLTIFY" {
		c.logger.Error().Err(err).Msgf("incorrect dest address or invalid destination")
		return nil, err
	}
	grpcClient, err := grpc.Dial(c.CosHandler.GrpcAddr, grpc.WithInsecure())
	if err != nil {
		zlog.Logger.Error().Err(err).Msgf("fail to dial at queue new block")
		return nil, err
	}
	defer grpcClient.Close()

	poolAddrs := []sdk.AccAddress{pools[0].CosAddress, pools[1].CosAddress}
	var items []*common.InBoundReq

	txClient := cosTx.NewServiceClient(grpcClient)
	for _, msg := range txWithMemo.GetMsgs() {
		switch eachMsg := msg.(type) {
		case *banktypes.MsgSend:
			item, err := c.processMsg(txBlockHeight, poolAddrs, txMemo, eachMsg, rawTx.Hash())
			if err != nil {
				if err.Error() != "not a top up message to the pool" {
					c.logger.Error().Err(err).Msgf("fail to process the message, it is not a top up message")
				}
				continue
			}

			txquery := cosTx.GetTxRequest{Hash: hex.EncodeToString(rawTx.Hash())}
			t, err := txClient.GetTx(ctx, &txquery)
			if err != nil {
				c.logger.Error().Err(err).Msgf("fail to query the tx")
				continue
			}
			if t.TxResponse.Code != 0 {
				// this means this tx is not a successful tx
				zlog.Warn().Msgf("not a valid inbound message with error code %v (%v)", t.TxResponse.Code, t.TxResponse.RawLog)
				continue
			}

			items = append(items, item)
		default:
			continue
		}
	}
	return items, nil
}
