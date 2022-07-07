package oppybridge

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	bcommon "gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/tssclient"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
)

func (oc *OppyChainInstance) processTopUpRequest(msg *banktypes.MsgSend, blockHeight int64, currEthAddr ethcommon.Address, memo OutBoundMemo) error {
	addr, tokenExist := oc.TokenList.GetTokenAddress(msg.Amount[0].GetDenom())
	if !tokenExist || msg.Amount[0].GetDenom() != config.OutBoundDenomFee {
		return errors.New("token is not on our token list or not fee demon")
	}
	tokenAddr := addr

	dat, ok := oc.pendingTx.LoadAndDelete(memo.TopupID)
	if !ok {
		return fmt.Errorf("saved tx not found invalid top up message %v", memo)
	}

	gasWanted, ok := new(big.Int).SetString(config.DefaultPUBChainGasWanted, 10)
	if !ok {
		panic("fail to load the gas wanted")
	}
	price := oc.GetPubChainGasPrice()
	expectedFeeAmount := new(big.Int).Mul(big.NewInt(price), gasWanted)
	expectedFee := types.NewCoin(config.OutBoundDenomFee, types.NewIntFromBigInt(expectedFeeAmount))

	savedTx := dat.(*OutboundTx)

	if savedTx.TokenAddr == config.NativeSign {
		savedTx.Token = savedTx.Token.AddAmount(msg.Amount[0].Amount)
		if !savedTx.Token.IsGTE(expectedFee) {
			oc.logger.Error().Msgf("the transaction is invalid,as fee we want is %v, and you have paid %v", expectedFee.String(), savedTx.Fee.String())
			oc.pendingTx.Store(memo.TopupID, savedTx)
			return nil
		}
		savedTx.Token = savedTx.Token.SubAmount(expectedFee.Amount)
	} else {
		// now we process the erc20 topup
		savedTx.Fee = savedTx.Fee.AddAmount(msg.Amount[0].Amount)
		if !savedTx.Fee.IsGTE(expectedFee) {
			oc.logger.Error().Msgf("the transaction is invalid,as fee we want is %v, and you have paid %v", expectedFee.String(), savedTx.Fee.String())
			oc.pendingTx.Store(memo.TopupID, savedTx)
			return nil
		}
	}
	roundBlockHeight := blockHeight / ROUNDBLOCK
	itemReq := bcommon.NewOutboundReq(memo.TopupID, savedTx.OutReceiverAddress, currEthAddr, savedTx.Token, tokenAddr, blockHeight, roundBlockHeight)
	oc.AddItem(&itemReq)
	oc.logger.Info().Msgf("Outbount Transaction in Block %v (Current Block %v)", blockHeight, oc.CurrentHeight)
	return nil
}

func (oc *OppyChainInstance) processNativeRequest(msg *banktypes.MsgSend, txID string, blockHeight int64, currEthAddr ethcommon.Address, memo OutBoundMemo) error {
	addr, tokenExist := oc.TokenList.GetTokenAddress(msg.Amount[0].GetDenom())
	if !tokenExist {
		return errors.New("token is not on our token list")
	}
	tokenDenom := msg.Amount[0].GetDenom()
	tokenAddr := addr

	if addr != config.NativeSign {
		return errors.New("not a native token")
	}

	item := oc.processNativeFee(txID, blockHeight, ethcommon.HexToAddress(memo.Dest), tokenDenom, msg.Amount[0].Amount)
	// since the cosmos address is different from the eth address, we need to derive the eth address from the public key
	if item != nil {
		roundBlockHeight := blockHeight / ROUNDBLOCK
		itemReq := bcommon.NewOutboundReq(txID, item.OutReceiverAddress, currEthAddr, item.Token, tokenAddr, blockHeight, roundBlockHeight)
		oc.AddItem(&itemReq)
		oc.logger.Info().Msgf("Outbount Transaction in Block %v (Current Block %v)", blockHeight, oc.CurrentHeight)
		return nil
	}
	return nil

}

func (oc *OppyChainInstance) processErc20Request(msg *banktypes.MsgSend, txID string, blockHeight int64, currEthAddr ethcommon.Address, memo OutBoundMemo) error {
	// now we search for the index of the outboundemo and the outbounddemofee
	found := false
	indexDemo := 0
	indexDemoFee := 0
	tokenDenom := ""
	tokenAddr := ""
	addr, tokenExist := oc.TokenList.GetTokenAddress(msg.Amount[0].GetDenom())
	if tokenExist && msg.Amount[1].GetDenom() == config.OutBoundDenomFee {
		tokenDenom = msg.Amount[0].GetDenom()
		tokenAddr = addr
		indexDemo = 0
		indexDemoFee = 1
		found = true
	}

	addr, tokenExist = oc.TokenList.GetTokenAddress(msg.Amount[1].GetDenom())
	if tokenExist && msg.Amount[0].GetDenom() == config.OutBoundDenomFee {
		tokenDenom = msg.Amount[1].GetDenom()
		tokenAddr = addr
		indexDemo = 1
		indexDemoFee = 0
		found = true
	}
	if !found {
		return errors.New("invalid fee pair")
	}

	item := oc.processErc20DemonAndFee(txID, blockHeight, ethcommon.HexToAddress(memo.Dest), tokenDenom, msg.Amount[indexDemo].Amount, msg.Amount[indexDemoFee].Amount)
	// since the cosmos address is different from the eth address, we need to derive the eth address from the public key
	if item != nil {
		roundBlockHeight := blockHeight / ROUNDBLOCK
		itemReq := bcommon.NewOutboundReq(txID, item.OutReceiverAddress, currEthAddr, item.Token, tokenAddr, blockHeight, roundBlockHeight)
		oc.AddItem(&itemReq)
		oc.logger.Info().Msgf("Outbount Transaction in Block %v (Current Block %v)", blockHeight, oc.CurrentHeight)
		return nil
	}
	return nil
}

// processMsg handle the oppychain transactions
func (oc *OppyChainInstance) processMsg(blockHeight int64, address []types.AccAddress, curEthAddr ethcommon.Address, memo OutBoundMemo, msg *banktypes.MsgSend, txHash []byte) error {
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
			err := oc.processTopUpRequest(msg, blockHeight, curEthAddr, memo)
			if err != nil {
				oc.logger.Error().Err(err).Msg("")
				return errors.New("fail to process the native token top up request")
			}
			return nil
		}
		err := oc.processNativeRequest(msg, txID, blockHeight, curEthAddr, memo)
		if err != nil {
			oc.logger.Error().Err(err).Msg("")
			return errors.New("fail to process the native token outbound request")
		}
		return nil
	case 2:
		err := oc.processErc20Request(msg, txID, blockHeight, curEthAddr, memo)
		if err != nil {
			return errors.New("fail to process the outbound erc20 request")
		}
		return nil
	default:
		return errors.New("incorrect msg format")
	}
}

func (oc *OppyChainInstance) processNativeFee(txID string, blockHeight int64, receiverAddr ethcommon.Address, demonName string, demonAmount types.Int) *OutboundTx {
	gasWanted, ok := new(big.Int).SetString(config.DefaultPUBChainGasWanted, 10)
	if !ok {
		panic("fail to load the gas wanted")
	}

	price := oc.GetPubChainGasPrice()
	expectedFeeAmount := new(big.Int).Mul(big.NewInt(price), gasWanted)
	expectedFee := types.NewCoin(config.OutBoundDenomFee, types.NewIntFromBigInt(expectedFeeAmount))

	tokenAddr, exit := oc.TokenList.GetTokenAddress(demonName)
	if !exit {
		oc.logger.Error().Msgf("The token is not existed in the white list")
		return nil
	}

	token := types.Coin{
		Denom:  demonName,
		Amount: demonAmount,
	}

	tx := OutboundTx{
		receiverAddr,
		uint64(blockHeight),
		token,
		tokenAddr,
		expectedFee,
		txID,
	}

	AmountTransfer := demonAmount.Sub(expectedFee.Amount)

	if AmountTransfer.IsNegative() {
		oc.logger.Warn().Msgf("The amount to transfer is smaller than the fee")
		oc.pendingTx.Store(txID, &tx)
		return nil
	}

	tx.Token = tx.Token.SubAmount(expectedFee.Amount)

	oc.logger.Info().Msgf("we add the outbound tokens tx(%v):%v", txID, tx.Token.String())
	return &tx

}

func (oc *OppyChainInstance) processErc20DemonAndFee(txID string, blockHeight int64, receiverAddr ethcommon.Address, demonName string, demonAmount, feeAmount types.Int) *OutboundTx {
	token := types.Coin{
		Denom:  demonName,
		Amount: demonAmount,
	}
	fee := types.Coin{
		Denom:  config.OutBoundDenomFee,
		Amount: feeAmount,
	}
	tokenAddr, exit := oc.TokenList.GetTokenAddress(demonName)
	if !exit {
		oc.logger.Error().Msgf("The token is not existed in the white list")
		return nil
	}
	tx := OutboundTx{
		receiverAddr,
		uint64(blockHeight),
		token,
		tokenAddr,
		fee,
		txID,
	}
	price := oc.GetPubChainGasPrice()

	gasWanted, ok := new(big.Int).SetString(config.DefaultPUBChainGasWanted, 10)
	if !ok {
		panic("fail to load the gas wanted")
	}
	expectedFeeAmount := new(big.Int).Mul(big.NewInt(price), gasWanted)
	expectedFee := types.NewCoin(config.OutBoundDenomFee, types.NewIntFromBigInt(expectedFeeAmount))

	if !fee.IsGTE(expectedFee) {
		oc.logger.Error().Msgf("the transaction is invalid,as fee we want is %v, and you have paid %v", expectedFee.String(), fee.String())
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

func (oc *OppyChainInstance) DoMoveFunds(fromPool *bcommon.PoolInfo, to types.AccAddress, height int64) (bool, error) {
	from := fromPool.OppyAddress
	acc, err := queryAccount(from.String(), oc.grpcClient)
	if err != nil {
		oc.logger.Error().Err(err).Msg("Fail to query the pool account")
		return false, err
	}
	coins, err := queryBalance(from.String(), oc.grpcClient)
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

	ok, resp, err := oc.composeAndSend(key, msg, acc.GetSequence(), acc.GetAccountNumber(), &signMsg, acc.GetAddress())
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
