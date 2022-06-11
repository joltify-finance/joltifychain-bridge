package oppybridge

import (
	"encoding/hex"
	"errors"
	"fmt"
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

func (jc *OppyChainInstance) processMsg(blockHeight int64, address []types.AccAddress, curEthAddr ethcommon.Address, msg *banktypes.MsgSend, txHash []byte) error {
	txID := strings.ToLower(hex.EncodeToString(txHash))

	toAddress, err := types.AccAddressFromBech32(msg.ToAddress)
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to parse the to outReceiverAddress")
		return err
	}

	// here we need to calculate the node's eth address from public key rather than the oppy chain address
	acc, err := queryAccount(msg.FromAddress, jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to query the account")
		return err
	}

	fromEthAddr, err := misc.AccountPubKeyToEthAddress(acc.GetPubKey())
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to get the eth address")
		return err
	}
	// now we wrap the fromEthAddress with oppy hex address
	wrapFromEthAddr, err := types.AccAddressFromHex(fromEthAddr.Hex()[2:])
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to wrap the eth address")
		return err
	}

	// we check whether it is the message to the pool
	if !(toAddress.Equals(address[0]) || toAddress.Equals(address[1])) {
		jc.logger.Warn().Msg("not a top up message to the pool")
		return errors.New("not a top up message to the pool")
	}

	// it means the sender pay the fee in one tx
	if len(msg.Amount) == 2 {
		// now we search for the index of the outboundemo and the outbounddemofee
		found := false
		indexDemo := 0
		indexDemoFee := 0
		tokenDenom := ""
		tokenAddr := ""
		addr, tokenExist := jc.TokenList.GetTokenAddress(msg.Amount[0].GetDenom())
		if tokenExist && msg.Amount[1].GetDenom() == config.OutBoundDenomFee {
			tokenDenom = msg.Amount[0].GetDenom()
			tokenAddr = addr
			indexDemo = 0
			indexDemoFee = 1
			found = true
		}

		addr, tokenExist = jc.TokenList.GetTokenAddress(msg.Amount[1].GetDenom())
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

		item := jc.processDemonAndFee(txID, blockHeight, wrapFromEthAddr, tokenDenom, msg.Amount[indexDemo].Amount, msg.Amount[indexDemoFee].Amount)
		// since the cosmos address is different from the eth address, we need to derive the eth address from the public key
		if item != nil {
			roundBlockHeight := blockHeight / ROUNDBLOCK
			itemReq := bcommon.NewOutboundReq(txID, item.outReceiverAddress, curEthAddr, item.token, tokenAddr, blockHeight, roundBlockHeight)
			jc.AddItem(&itemReq)
			jc.logger.Info().Msgf("Outbount Transaction in Block %v (Current Block %v)", blockHeight, jc.CurrentHeight)
			return nil
		}
		return errors.New("not enough fee")
	}

	return errors.New("we only allow fee and top up in one tx now")
}

func (jc *OppyChainInstance) processDemonAndFee(txID string, blockHeight int64, fromAddress types.AccAddress, demonName string, demonAmount, feeAmount types.Int) *outboundTx {
	token := types.Coin{
		Denom:  demonName,
		Amount: demonAmount,
	}
	fee := types.Coin{
		Denom:  config.OutBoundDenomFee,
		Amount: feeAmount,
	}
	tokenAddr, exit := jc.TokenList.GetTokenAddress(demonName)
	if !exit {
		jc.logger.Error().Msgf("The token is not existed in the white list")
		return nil
	}
	tx := outboundTx{
		ethcommon.BytesToAddress(fromAddress.Bytes()),
		uint64(blockHeight),
		token,
		tokenAddr,
		fee,
	}
	err := tx.Verify()
	if err != nil {
		jc.logger.Error().Err(err).Msgf("the transaction is invalid")
		return nil
	}
	jc.logger.Info().Msgf("we add the outbound tokens tx(%v):%v", txID, tx.token.String())
	return &tx
}

// GetPool get the latest two pool address
func (jc *OppyChainInstance) GetPool() []*bcommon.PoolInfo {
	jc.poolUpdateLocker.RLock()
	defer jc.poolUpdateLocker.RUnlock()
	var ret []*bcommon.PoolInfo
	ret = append(ret, jc.lastTwoPools...)
	return ret
}

// UpdatePool update the tss pool address
func (jc *OppyChainInstance) UpdatePool(pool *vaulttypes.PoolInfo) *bcommon.PoolInfo {
	poolPubKey := pool.CreatePool.PoolPubKey
	ethAddr, err := misc.PoolPubKeyToEthAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the jolt address to eth address %v", poolPubKey)
		return nil
	}

	addr, err := misc.PoolPubKeyToJoltAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the jolt address to jolt address %v", poolPubKey)
		return nil
	}

	p := bcommon.PoolInfo{
		Pk:          poolPubKey,
		OppyAddress: addr,
		EthAddress:  ethAddr,
		PoolInfo:    pool,
	}

	jc.poolUpdateLocker.Lock()
	previousPool := jc.lastTwoPools[0]

	if jc.lastTwoPools[1] != nil {
		jc.lastTwoPools[0] = jc.lastTwoPools[1]
	}
	jc.lastTwoPools[1] = &p
	jc.poolUpdateLocker.Unlock()
	return previousPool
}

func (jc *OppyChainInstance) DoMoveFunds(fromPool *bcommon.PoolInfo, to types.AccAddress, height int64) (bool, error) {
	from := fromPool.OppyAddress
	acc, err := queryAccount(from.String(), jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to query the pool account")
		return false, err
	}
	coins, err := queryBalance(from.String(), jc.grpcClient)
	if err != nil {
		jc.logger.Error().Err(err).Msg("Fail to query the balance")
		return false, err
	}
	if len(coins) == 0 {
		jc.logger.Warn().Msg("we do not have any balance skip send")
		return true, nil
	}

	msg := banktypes.NewMsgSend(from, to, coins)

	signMsg := tssclient.TssSignigMsg{
		Pk:          fromPool.Pk,
		Signers:     nil,
		BlockHeight: height,
		Version:     tssclient.TssVersion,
	}

	key, err := jc.Keyring.Key("operator")
	if err != nil {
		jc.logger.Error().Err(err).Msg("fail to get the operator key")
		return false, err
	}

	ok, resp, err := jc.composeAndSend(key, msg, acc.GetSequence(), acc.GetAccountNumber(), &signMsg, acc.GetAddress())
	if err != nil || !ok {
		jc.logger.Error().Err(err).Msgf("fail to broadcast the tx->%v", resp)
		return false, errors.New("fail to process the inbound tx")
	}
	return false, nil
}

// Verify checks whether the outbound tx has paid enough fee
func (a *outboundTx) Verify() error {
	if a.fee.Denom != config.OutBoundDenomFee {
		return errors.New("invalid outbound fee denom")
	}
	amount, err := types.NewDecFromStr(config.OUTBoundFeeOut)
	if err != nil {
		return errors.New("invalid minimal inbound fee")
	}
	if a.fee.Amount.LT(types.NewIntFromBigInt(amount.BigInt())) {
		return fmt.Errorf("the fee is not enough with %s<%s", a.fee.Amount, amount.BigInt().String())
	}
	return nil
}
