package pubchain

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"

	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

// ProcessInBound process the inbound contract token top-up
func (pi *PubChainInstance) ProcessInBound(transfer *TokenTransfer) error {
	if transfer.Raw.Removed {
		return errors.New("the tx is the revert tx")
	}

	tokenAddr := transfer.Raw.Address
	tx, isPending, err := pi.EthClient.TransactionByHash(context.Background(), transfer.Raw.TxHash)
	if err != nil || isPending {
		pi.logger.Error().Err(err).Msg("fail to get this transaction.")
		return err
	}
	if isPending {
		return fmt.Errorf("pending transaction with hash id %v", transfer.Raw.TxHash.String())
	}
	v, r, s := tx.RawSignatureValues()
	signer := ethTypes.LatestSignerForChainID(tx.ChainId())
	plainV := misc.RecoverRecID(tx.ChainId().Uint64(), v)
	sigBytes := misc.MakeSignature(r, s, plainV)
	fmt.Printf("#############%v\n", plainV.String())

	sigPublicKey, err := crypto.Ecrecover(signer.Hash(tx).Bytes(), sigBytes)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to recover the public key")
		return err
	}

	transferFrom, err := misc.EthSignPubKeyToJoltAddr(sigPublicKey)
	fmt.Printf(">>>>>>>>transfer from %v\n", transferFrom.String())

	pubkeystrc, err := crypto.UnmarshalPubkey(sigPublicKey)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to recover the public key eth")
		return err
	}
	address := crypto.PubkeyToAddress(*pubkeystrc)
	fmt.Printf(">>>>>eth address %v and should be %v\n", address, transfer.From.String())

	if plainV.Uint64() == 1 {
		plainV = big.NewInt(0)
	} else {
		plainV = big.NewInt(1)
	}

	sigBytes = misc.MakeSignature(r, s, plainV)

	sigPublicKey2, err := crypto.Ecrecover(transfer.Raw.TxHash.Bytes(), sigBytes)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to recover the public key")
		return err
	}

	pubkeystrc2, err := crypto.UnmarshalPubkey(sigPublicKey2)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to recover the public key eth")
		return err
	}
	address2 := crypto.PubkeyToAddress(*pubkeystrc2)
	fmt.Printf(">>>>>eth2 address %v and should be %v\n", address2, transfer.From.String())

	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to get the joltify address from the public key recovered from sig")
		return err
	}
	err = pi.processInboundTx(transfer.Raw.TxHash.Hex()[2:], transfer.Raw.BlockNumber, transferFrom, transfer.Value, tokenAddr)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to process the inbound tx")
	}
	return err
}

// ProcessNewBlock process the blocks received from the public pub_chain
func (pi *PubChainInstance) ProcessNewBlock(number *big.Int) error {
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	block, err := pi.EthClient.BlockByNumber(ctx, number)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to retrieve the block")
		return err
	}
	pi.processEachBlock(block)
	return nil
}

// updateInboundTx update the top-up token with fee
func (pi *PubChainInstance) updateInboundTx(txID string, amount *big.Int) *inboundTx {
	pi.pendingInboundTxLocker.Lock()
	defer pi.pendingInboundTxLocker.Unlock()
	thisAccount, ok := pi.pendingInbounds[txID]
	if !ok {
		pi.logger.Warn().Msgf("fail to get the stored tx from pool with %v\n", pi.pendingInbounds)
		return nil
	}

	thisAccount.fee.Amount = thisAccount.fee.Amount.Add(types.NewIntFromBigInt(amount))
	err := thisAccount.Verify()
	if err != nil {
		pi.pendingInbounds[txID] = thisAccount
		pi.logger.Warn().Msgf("the account cannot be processed on joltify pub_chain this round with err %v\n", err)
		return nil
	}
	// since this tx is processed,we do not need to store it any longer
	delete(pi.pendingInbounds, txID)
	return thisAccount
}

func (pi *PubChainInstance) processInboundTx(txID string, blockHeight uint64, from types.AccAddress, value *big.Int, addr common.Address) error {
	pi.pendingInboundTxLocker.Lock()
	defer pi.pendingInboundTxLocker.Unlock()
	_, ok := pi.pendingInbounds[txID]
	if ok {
		pi.logger.Error().Msgf("the tx already exist!!")
		return errors.New("tx existed")
	}

	if addr.String() != pi.tokenAddr {
		pi.logger.Error().Msgf("incorrect top up token")
		return errors.New("incorrect top up token")
	}

	token := types.Coin{
		Denom:  config.InBoundDenom,
		Amount: types.NewIntFromBigInt(value),
	}
	fee := types.Coin{
		Denom:  config.InBoundDenomFee,
		Amount: types.NewInt(0),
	}

	tx := inboundTx{
		from,
		blockHeight,
		token,
		fee,
	}
	pi.logger.Info().Msgf("we add the tokens tx(%v):%v", txID, tx.token.String())
	pi.pendingInbounds[txID] = &tx
	return nil
}

// fixme we need to check timeout to remove the pending transactions
func (pi *PubChainInstance) processEachBlock(block *ethTypes.Block) {
	for _, tx := range block.Transactions() {
		if tx.To() == nil || tx.Value() == nil {
			continue
		}
		if pi.checkToBridge(*tx.To()) {
			if tx.Data() == nil {
				pi.logger.Warn().Msgf("we have received unknown fund")
				continue
			}
			payTxID := tx.Data()
			account := pi.updateInboundTx(hex.EncodeToString(payTxID), tx.Value())
			if account != nil {
				item := newAccountInboundReq(account.address, *tx.To(), account.token, block.Number().Int64())
				pi.InboundReqChan <- &item
			}
		}
	}
}

// UpdatePool update the tss pool address
func (pi *PubChainInstance) UpdatePool(poolPubKey string) {
	addr, err := misc.PoolPubKeyToJoltAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the jolt address to eth address %v", poolPubKey)
		return
	}

	ethAddr, err := misc.PoolPubKeyToEthAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the jolt address to eth address %v", poolPubKey)
		return
	}

	pi.poolLocker.Lock()
	defer pi.poolLocker.Unlock()

	p := bcommon.PoolInfo{
		Pk:             poolPubKey,
		JoltifyAddress: addr,
		EthAddress:     ethAddr,
	}

	if pi.lastTwoPools[1] != nil {
		pi.lastTwoPools[0] = pi.lastTwoPools[1]
	}
	pi.lastTwoPools[1] = &p
	pi.UpdateSubscribe(pi.lastTwoPools)
	return
}

// GetPool get the latest two pool address
func (pi *PubChainInstance) GetPool() []*bcommon.PoolInfo {
	pi.poolLocker.RLock()
	defer pi.poolLocker.RUnlock()
	var ret []*bcommon.PoolInfo
	ret = append(ret, pi.lastTwoPools...)
	return ret
}

// GetPool get the latest two pool address
func (pi *PubChainInstance) checkToBridge(dest common.Address) bool {
	pools := pi.GetPool()
	for _, el := range pools {
		if el != nil && dest.String() == el.EthAddress.String() {
			return true
		}
	}
	return false
}

// DeleteExpired delete the expired tx
func (pi *PubChainInstance) DeleteExpired(currentHeight uint64) {
	pi.pendingInboundTxLocker.Lock()
	defer pi.pendingInboundTxLocker.Unlock()
	var expiredTx []string
	for key, el := range pi.pendingInbounds {
		if currentHeight-el.blockHeight > config.TxTimeout {
			expiredTx = append(expiredTx, key)
		}
	}
	for _, el := range expiredTx {
		pi.logger.Warn().Msgf("we delete the expired tx %s", el)
		delete(pi.pendingInbounds, el)
	}
}
