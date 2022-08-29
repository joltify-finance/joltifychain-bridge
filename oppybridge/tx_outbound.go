package oppybridge

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	grpc1 "github.com/gogo/protobuf/grpc"
	bcommon "gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/tssclient"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
)

func (oc *OppyChainInstance) calculateGas() types.Coin {
	fee := oc.GetPubChainFee()
	expectedFee := types.NewCoin(config.OutBoundDenomFee, types.NewIntFromUint64(uint64(fee)))
	return expectedFee
}

func (oc *OppyChainInstance) processTopUpRequest(msg *banktypes.MsgSend, txBlockHeight int64, currEthAddr ethcommon.Address, memo bcommon.BridgeMemo) error {
	tokenItem, tokenExist := oc.TokenList.GetTokenInfoByDenom(msg.Amount[0].GetDenom())
	if !tokenExist || msg.Amount[0].GetDenom() != config.OutBoundDenomFee {
		return errors.New("token is not on our token list or not fee demon")
	}

	dat, ok := oc.pendingTx.LoadAndDelete(memo.TopupID)
	if !ok {
		return fmt.Errorf("saved tx not found invalid top up message %v", memo)
	}
	savedTx := dat.(*OutboundTx)
	expectedFee := savedTx.FeeWanted

	if savedTx.TokenAddr == config.NativeSign {
		oc.logger.Warn().Msgf("topping up native token is not supported")
	} else {
		// now we process the erc20 topup
		savedTx.Fee = savedTx.Fee.AddAmount(msg.Amount[0].Amount)
		if !savedTx.Fee.IsGTE(expectedFee) {
			oc.logger.Error().Msgf("the transaction is invalid,as fee we want is %v, and you have paid %v", expectedFee.String(), savedTx.Fee.String())
			oc.pendingTx.Store(memo.TopupID, savedTx)
			return nil
		}
	}
	// we need to adjust the decimal as some token may not have 18 decimals
	// in oppy, we apply 18 decimal
	delta := tokenItem.Decimals - types.Precision
	if delta != 0 {
		adjustedTokenAmount := bcommon.AdjustInt(savedTx.Token.Amount, int64(delta))
		savedTx.Token.Amount = adjustedTokenAmount
	}

	itemReq := bcommon.NewOutboundReq(memo.TopupID, savedTx.OutReceiverAddress, currEthAddr, savedTx.Token, savedTx.TokenAddr, txBlockHeight)
	oc.AddItem(&itemReq)
	oc.logger.Info().Msgf("Outbound Transaction in Block %v (Current Block %v) to %s with amount %v", txBlockHeight, oc.CurrentHeight, savedTx.OutReceiverAddress, savedTx.Token)
	return nil
}

func (oc *OppyChainInstance) processNativeRequest(msg *banktypes.MsgSend, txID string, txBlockHeight int64, currEthAddr ethcommon.Address, memo bcommon.BridgeMemo) error {
	tokenItem, tokenExist := oc.TokenList.GetTokenInfoByDenom(msg.Amount[0].GetDenom())
	if !tokenExist {
		return errors.New("token is not on our token list")
	}
	tokenDenom := msg.Amount[0].GetDenom()
	tokenAddr := tokenItem.TokenAddr

	if tokenAddr != config.NativeSign {
		return errors.New("not a native token")
	}

	item := oc.processNativeFee(txID, msg.FromAddress, tokenAddr, txBlockHeight, ethcommon.HexToAddress(memo.Dest), tokenDenom, msg.Amount[0].Amount)
	// since the cosmos address is different from the eth address, we need to derive the eth address from the public key
	if item != nil {
		delta := tokenItem.Decimals - types.Precision
		if delta != 0 {
			adjustedTokenAmount := bcommon.AdjustInt(item.Token.Amount, int64(delta))
			item.Token.Amount = adjustedTokenAmount
		}
		itemReq := bcommon.NewOutboundReq(txID, item.OutReceiverAddress, currEthAddr, item.Token, tokenAddr, txBlockHeight)
		oc.AddItem(&itemReq)
		oc.logger.Info().Msgf("Outbount Transaction in Block %v (Current Block %v)", txBlockHeight, oc.CurrentHeight)
		return nil
	}
	return nil
}

func (oc *OppyChainInstance) processErc20Request(msg *banktypes.MsgSend, txID string, txBlockHeight int64, currEthAddr ethcommon.Address, memo bcommon.BridgeMemo) error {
	// now we search for the index of the outboundemo and the outbounddemofee
	found := false
	indexDemo := 0
	indexDemoFee := 0
	tokenDenom := ""
	tokenAddr := ""
	tokenItem, tokenExist := oc.TokenList.GetTokenInfoByDenom(msg.Amount[0].GetDenom())
	if tokenExist && msg.Amount[1].GetDenom() == config.OutBoundDenomFee {
		tokenDenom = msg.Amount[0].GetDenom()
		tokenAddr = tokenItem.TokenAddr
		indexDemo = 0
		indexDemoFee = 1
		found = true
	}

	tokenItem, tokenExist = oc.TokenList.GetTokenInfoByDenom(msg.Amount[1].GetDenom())
	if tokenExist && msg.Amount[0].GetDenom() == config.OutBoundDenomFee {
		tokenDenom = msg.Amount[1].GetDenom()
		tokenAddr = tokenItem.TokenAddr
		indexDemo = 1
		indexDemoFee = 0
		found = true
	}
	if !found {
		return errors.New("invalid fee pair")
	}

	item := oc.processErc20DemonAndFee(txID, msg.FromAddress, tokenAddr, txBlockHeight, ethcommon.HexToAddress(memo.Dest), tokenDenom, msg.Amount[indexDemo].Amount, msg.Amount[indexDemoFee].Amount)
	// since the cosmos address is different from the eth address, we need to derive the eth address from the public key
	if item != nil {
		delta := tokenItem.Decimals - types.Precision
		if delta != 0 {
			adjustedTokenAmount := bcommon.AdjustInt(item.Token.Amount, int64(delta))
			item.Token.Amount = adjustedTokenAmount
		}
		itemReq := bcommon.NewOutboundReq(txID, item.OutReceiverAddress, currEthAddr, item.Token, tokenAddr, txBlockHeight)
		oc.AddItem(&itemReq)
		oc.logger.Info().Msgf("Outbound Transaction in Block %v (Current Block %v)", txBlockHeight, oc.CurrentHeight)
		return nil
	}
	return nil
}

// processMsg handle the oppychain transactions
func (oc *OppyChainInstance) processMsg(txBlockHeight int64, address []types.AccAddress, curEthAddr ethcommon.Address, memo bcommon.BridgeMemo, msg *banktypes.MsgSend, txHash []byte) error {
	txID := strings.ToLower(hex.EncodeToString(txHash))

	toAddress, err := types.AccAddressFromBech32(msg.ToAddress)
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to parse the to outReceiverAddress")
		return err
	}

	// we check whether it is the message to the pool
	if !(toAddress.Equals(address[0]) || toAddress.Equals(address[1])) {
		oc.logger.Warn().Msg("not a top up message to the pool")
		return errors.New("not a top up message to the pool")
	}

	// it means the sender pay the fee in one tx
	switch len(msg.Amount) {
	case 1:
		if len(memo.TopupID) != 0 {
			err := oc.processTopUpRequest(msg, txBlockHeight, curEthAddr, memo)
			if err != nil {
				oc.logger.Error().Err(err).Msg("")
				return errors.New("fail to process the native token top up request")
			}
			return nil
		}
		err := oc.processNativeRequest(msg, txID, txBlockHeight, curEthAddr, memo)
		if err != nil {
			oc.logger.Error().Err(err).Msg("")
			return errors.New("fail to process the native token outbound request")
		}
		return nil
	case 2:
		err := oc.processErc20Request(msg, txID, txBlockHeight, curEthAddr, memo)
		if err != nil {
			return errors.New("fail to process the outbound erc20 request")
		}
		return nil
	default:
		return errors.New("incorrect msg format")
	}
}

func (oc *OppyChainInstance) processNativeFee(txID, fromAddress string, tokenAddr string, txBlockHeight int64, receiverAddr ethcommon.Address, demonName string, demonAmount types.Int) *OutboundTx {
	expectedFee := oc.calculateGas()

	token := types.Coin{
		Denom:  demonName,
		Amount: demonAmount,
	}

	tx := OutboundTx{
		receiverAddr,
		fromAddress,
		// todo should be dynamic once we have more than one chain
		"56",
		uint64(txBlockHeight),
		token,
		tokenAddr,
		expectedFee,
		expectedFee,
		txID,
	}

	AmountTransfer := demonAmount.Sub(expectedFee.Amount)

	if AmountTransfer.IsNegative() {
		oc.logger.Warn().Msgf("The amount to transfer is smaller than the fee %v, we drop this tx", expectedFee.String())
		return nil
	}
	tx.Token = tx.Token.SubAmount(expectedFee.Amount)
	oc.logger.Info().Msgf("we add the outbound tokens tx(%v):%v", txID, tx.Token.String())
	return &tx
}

func (oc *OppyChainInstance) processErc20DemonAndFee(txID, fromAddress string, tokenAddr string, blockHeight int64, receiverAddr ethcommon.Address, demonName string, demonAmount, feeAmount types.Int) *OutboundTx {
	token := types.Coin{
		Denom:  demonName,
		Amount: demonAmount,
	}
	fee := types.Coin{
		Denom:  config.OutBoundDenomFee,
		Amount: feeAmount,
	}

	expectedFee := oc.calculateGas()
	tx := OutboundTx{
		receiverAddr,
		fromAddress,
		// todo should be dynamic once we have another chain
		"56",
		uint64(blockHeight),
		token,
		tokenAddr,
		fee,
		expectedFee,
		txID,
	}

	if !fee.IsGTE(expectedFee) {
		oc.logger.Error().Msgf("the outbound transaction is invalid ,as fee we want is %v, and you have paid %v", expectedFee.String(), fee.String())
		oc.pendingTx.Store(txID, &tx)
		return nil
	}
	oc.logger.Info().Msgf("we add the outbound tokens tx(%v):%v", txID, tx.Token.String())
	return &tx
}

// GetPool get the latest two pool address
func (oc *OppyChainInstance) GetPool() []*bcommon.PoolInfo {
	oc.poolUpdateLocker.RLock()
	defer oc.poolUpdateLocker.RUnlock()
	var ret []*bcommon.PoolInfo
	ret = append(ret, oc.lastTwoPools...)
	return ret
}

// UpdatePool update the tss pool address
func (oc *OppyChainInstance) UpdatePool(pool *vaulttypes.PoolInfo) *bcommon.PoolInfo {
	poolPubKey := pool.CreatePool.PoolPubKey
	ethAddr, err := misc.PoolPubKeyToEthAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the oppy address to eth address %v", poolPubKey)
		return nil
	}

	addr, err := misc.PoolPubKeyToOppyAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the eth address to oppy address %v", poolPubKey)
		return nil
	}

	p := bcommon.PoolInfo{
		Pk:          poolPubKey,
		OppyAddress: addr,
		EthAddress:  ethAddr,
		PoolInfo:    pool,
	}

	oc.poolUpdateLocker.Lock()
	previousPool := oc.lastTwoPools[0]

	if oc.lastTwoPools[1] != nil {
		oc.lastTwoPools[0] = oc.lastTwoPools[1]
	}
	oc.lastTwoPools[1] = &p
	oc.poolUpdateLocker.Unlock()
	return previousPool
}

func (oc *OppyChainInstance) DoMoveFunds(conn grpc1.ClientConn, fromPool *bcommon.PoolInfo, to types.AccAddress, height int64) (bool, error) {
	from := fromPool.OppyAddress
	acc, err := queryAccount(conn, from.String(), "")
	if err != nil {
		oc.logger.Error().Err(err).Msg("Fail to query the pool account")
		return false, err
	}
	coins, err := queryBalance(from.String(), oc.GrpcClient)
	if err != nil {
		oc.logger.Error().Err(err).Msg("Fail to query the balance")
		return false, err
	}
	if len(coins) == 0 {
		oc.logger.Warn().Msg("we do not have any balance skip send")
		return true, nil
	}

	msg := banktypes.NewMsgSend(from, to, coins)

	signMsg := tssclient.TssSignigMsg{
		Pk:          fromPool.Pk,
		Signers:     nil,
		BlockHeight: height,
		Version:     tssclient.TssVersion,
	}

	key, err := oc.Keyring.Key("operator")
	if err != nil {
		oc.logger.Error().Err(err).Msg("fail to get the operator key")
		return false, err
	}

	ok, resp, err := oc.composeAndSend(conn, key, msg, acc.GetSequence(), acc.GetAccountNumber(), &signMsg, acc.GetAddress())
	if err != nil || !ok {
		oc.logger.Error().Err(err).Msgf("fail to broadcast the tx->%v", resp)
		return false, errors.New("fail to process the inbound tx")
	}
	return false, nil
}

// DeleteExpired remove the expired pending tx
func (oc *OppyChainInstance) DeleteExpired(currentHeight int64) {
	var expiredTx []string
	oc.pendingTx.Range(func(key, value interface{}) bool {
		el := value.(*OutboundTx)
		if currentHeight-int64(el.BlockHeight) > config.TxTimeout {
			expiredTx = append(expiredTx, key.(string))
		}
		return true
	})

	for _, el := range expiredTx {
		oc.logger.Warn().Msgf("we delete the expired tx %s", el)
		oc.pendingTx.Delete(el)
	}
}
