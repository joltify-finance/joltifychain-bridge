package cosbridge

import (
	"context"
	"errors"
	"sync"

	"github.com/cosmos/cosmos-sdk/client"
	zlog "github.com/rs/zerolog/log"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	cosTx "github.com/cosmos/cosmos-sdk/types/tx"
	grpc1 "github.com/gogo/protobuf/grpc"
	"google.golang.org/grpc"

	"github.com/cenkalti/backoff"
	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	pricefeedtypes "github.com/joltify-finance/joltify_lending/x/third_party/pricefeed/types"
	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	"github.com/tendermint/tendermint/proto/tendermint/types"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
)

// queryAccount get the current sender Account info
func queryAccount(grpcClient grpc1.ClientConn, addr, grpcAddr string) (authtypes.AccountI, error) {
	var err error
	if grpcClient == nil {
		grpcClie2, err := grpc.Dial(grpcAddr, grpc.WithInsecure())
		if err != nil {
			return nil, err
		}
		defer grpcClie2.Close()
		grpcClient = grpcClie2
	}

	accQuery := authtypes.NewQueryClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	accResp, err := accQuery.Account(ctx, &authtypes.QueryAccountRequest{Address: addr})
	if err != nil {
		return nil, err
	}

	encCfg := MakeEncodingConfig()
	var acc authtypes.AccountI
	if err := encCfg.InterfaceRegistry.UnpackAny(accResp.Account, &acc); err != nil {
		return nil, err
	}
	return acc, nil
}

// queryLastValidatorSet get the last two validator sets
func queryLastValidatorSet(grpcClient grpc1.ClientConn) ([]*vaulttypes.PoolInfo, error) {
	ts := vaulttypes.NewQueryClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	req := vaulttypes.QueryLatestPoolRequest{}
	resp, err := ts.GetLastPool(ctx, &req)
	if err != nil {
		return nil, err
	}

	return resp.Pools, nil
}

func QueryTokenPrice(grpcClient grpc1.ClientConn, grpcAddr string, denom string) (sdk.Dec, error) {
	if grpcClient == nil {
		grpcClient2, err := grpc.Dial(grpcAddr, grpc.WithInsecure())
		if err != nil {
			zlog.Logger.Error().Err(err).Msgf("fail to dial the grpc end-point")
			return sdk.Dec{}, err
		}
		defer grpcClient2.Close()
		grpcClient = grpcClient2
	}

	qs := pricefeedtypes.NewQueryClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	var marketID string
	if denom == "ujolt" {
		marketID = "jolt:usd"
	} else {
		marketID = denom[1:] + ":usd"
	}
	req := pricefeedtypes.QueryPriceRequest{MarketId: marketID}

	result, err := qs.Price(ctx, &req)
	if err != nil {
		return sdk.Dec{}, err
	}
	return result.Price.Price, nil
}

// queryLastValidatorSet get the last two validator sets
func queryGivenToeknIssueTx(grpcClient grpc1.ClientConn, index string) (*vaulttypes.IssueToken, error) {
	ts := vaulttypes.NewQueryClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	req := vaulttypes.QueryGetIssueTokenRequest{
		Index: index,
	}
	resp, err := ts.IssueToken(ctx, &req)
	if err != nil {
		return nil, err
	}

	return resp.IssueToken, nil
}

// QueryTipValidator get the validator set of the tip of the current pub_chain
func QueryTipValidator(grpcClient grpc1.ClientConn) (int64, []*tmservice.Validator, error) {
	ts := tmservice.NewServiceClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	resp, err := ts.GetLatestValidatorSet(ctx, &tmservice.GetLatestValidatorSetRequest{})
	if err != nil {
		return 0, nil, err
	}

	return resp.BlockHeight, resp.Validators, nil
}

// GetLastBlockHeight get the last height of the joltify chain
func GetLastBlockHeight(grpcClient grpc1.ClientConn) (int64, error) {
	ts := tmservice.NewServiceClient(grpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	resp, err := ts.GetLatestBlock(ctx, &tmservice.GetLatestBlockRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Block.Header.Height, nil
}

// GetBlockByHeight get the block from joltify chain based on provided height
func GetBlockByHeight(grpcClient grpc1.ClientConn, height int64) (*types.Block, error) {
	ts := tmservice.NewServiceClient(grpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()
	req := tmservice.GetBlockByHeightRequest{
		Height: height,
	}
	resp, err := ts.GetBlockByHeight(ctx, &req)
	if err != nil {
		return nil, err
	}
	return resp.Block, nil
}

// CheckTxStatus check whether the tx has been done successfully
func (jc *JoltChainInstance) waitAndSend(conn grpc1.ClientConn, poolAddress sdk.AccAddress, targetSeq uint64) error {
	bf := backoff.WithMaxRetries(backoff.NewConstantBackOff(submitBackoff), 40)

	alreadyPassed := false
	op := func() error {
		acc, err := queryAccount(conn, poolAddress.String(), jc.grpcAddr)
		if err != nil {
			jc.logger.Error().Err(err).Msgf("fail to query the Account")
			return errors.New("invalid Account query")
		}
		if acc.GetSequence() == targetSeq {
			return nil
		}
		if acc.GetSequence() > targetSeq {
			alreadyPassed = true
			return nil
		}
		return errors.New("not our round")
	}

	err := backoff.Retry(op, bf)
	if alreadyPassed {
		return errors.New("already passed")
	}
	return err
}

func (jc *JoltChainInstance) batchComposeAndSend(conn grpc1.ClientConn, sendMsg []sdk.Msg, accSeq, accNum uint64, signMsg *tssclient.TssSignigMsg, poolAddress sdk.AccAddress) (map[uint64]string, error) {
	gasWanted, err := jc.GasEstimation(conn, sendMsg, accSeq, nil)
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to get the gas estimation")
		return nil, err
	}

	txBuilderSeqMap, err := jc.batchGenSendTx(sendMsg, accSeq, accNum, gasWanted, signMsg)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to generate the tx")
		return nil, err
	}

	wg := sync.WaitGroup{}
	txHashes := make(map[uint64]string)
	txHashesLocker := &sync.RWMutex{}
	for seq, el := range txBuilderSeqMap {
		if el == nil {
			jc.logger.Error().Msgf("the seq %v has nil tx builder!!", seq)
			txHashes[seq] = ""
			continue
		}
		wg.Add(1)
		go func(accSeq uint64, txBuilder client.TxBuilder) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
			defer cancel()

			txBytes, err := jc.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
			if err != nil {
				jc.logger.Error().Err(err).Msg("fail to encode the tx")
				txHashesLocker.Lock()
				txHashes[accSeq] = ""
				txHashesLocker.Unlock()
				return
			}

			err = jc.waitAndSend(conn, poolAddress, accSeq)
			if err == nil {
				_, resp, err := jc.BroadcastTx(ctx, conn, txBytes, true)
				if err != nil {
					jc.logger.Error().Err(err).Msg("fail to broadcast the signature")
				}
				txHashesLocker.Lock()
				txHashes[accSeq] = resp
				txHashesLocker.Unlock()
				return
			}
		}(seq, el)
	}
	wg.Wait()
	return txHashes, nil
}

func (jc *JoltChainInstance) composeAndSend(conn grpc1.ClientConn, operator keyring.Info, sendMsg sdk.Msg, accSeq, accNum uint64, signMsg *tssclient.TssSignigMsg, poolAddress sdk.AccAddress) (bool, string, error) {
	gasWanted, err := jc.GasEstimation(conn, []sdk.Msg{sendMsg}, accSeq, nil)
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to get the gas estimation")
		return false, "", err
	}

	txBuilder, err := jc.genSendTx(operator, []sdk.Msg{sendMsg}, accSeq, accNum, gasWanted, signMsg)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to generate the tx")
		return false, "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	txBytes, err := jc.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to encode the tx")
		return false, "", err
	}

	err = nil
	if signMsg != nil {
		err = jc.waitAndSend(conn, poolAddress, accSeq)
	}
	if err == nil {
		isTssMsg := true
		if signMsg == nil {
			isTssMsg = false
		}
		ok, resp, err := jc.BroadcastTx(ctx, conn, txBytes, isTssMsg)
		return ok, resp, err
	}
	return false, "", err
}

// BroadcastTx broadcast the tx to the oppyChain
func (jc *JoltChainInstance) BroadcastTx(ctx context.Context, conn grpc1.ClientConn, txBytes []byte, isTssMsg bool) (bool, string, error) {
	// Broadcast the tx via gRPC. We create a new client for the Protobuf Tx
	// service.
	txClient := cosTx.NewServiceClient(conn)
	// We then call the BroadcastTx method on this client.
	grpcRes, err := txClient.BroadcastTx(
		ctx,
		&cosTx.BroadcastTxRequest{
			Mode:    cosTx.BroadcastMode_BROADCAST_MODE_BLOCK,
			TxBytes: txBytes, // Proto-binary of the signed transaction, see previous step.
		},
	)
	if err != nil {
		return false, "", err
	}

	// this mean tx has been submitted by others
	if grpcRes.GetTxResponse().Code == 19 {
		return true, "", nil
	}

	if grpcRes.GetTxResponse().Code != 0 {
		jc.logger.Error().Err(err).Msgf("fail to broadcast with response %v", grpcRes.TxResponse)
		return false, "", nil
	}
	txHash := grpcRes.GetTxResponse().TxHash

	return true, txHash, nil
}

func outboundAdjust(amount sdk.Int, decimals int, precision int) sdk.Int {
	delta := decimals - precision
	if delta != 0 {
		adjustedTokenAmount := bcommon.AdjustInt(amount, int64(delta))
		return adjustedTokenAmount
	}
	return amount
}
