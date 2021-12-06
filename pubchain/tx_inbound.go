package pubchain

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"math/big"
)

// ProcessInBound process the inbound contract token top-up
func (pi *PubChainInstance) ProcessInBound(transfer *TokenTransfer) error {
	if transfer.Raw.Removed {
		return errors.New("the tx is the revert tx")
	}
	tokenAddr := transfer.Raw.Address
	err := pi.processInboundTx(transfer.Raw.TxHash.Hex()[2:], transfer.Raw.BlockNumber, transfer.From, transfer.Value, tokenAddr)
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
func (pi *PubChainInstance) updateInboundTx(txID string, amount *big.Int, direction config.Direction) *inboundTx {
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
		pi.logger.Warn().Msgf("the account cannot be processed on joltify pub_chain this round")
		return nil
	}
	// since this tx is processed,we do not need to store it any longer
	delete(pi.pendingInbounds, txID)
	return thisAccount
}

func (pi *PubChainInstance) processInboundTx(txID string, blockHeight uint64, from common.Address, value *big.Int, addr common.Address) error {
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
			account := pi.updateInboundTx(hex.EncodeToString(payTxID), tx.Value(), config.InBound)
			if account != nil {
				item := newAccountInboundReq(account.address, *tx.To(), account.token, block.Number().Int64())
				pi.InboundReqChan <- &item
			}
		}
	}
}

// UpdatePool update the tss pool address
func (pi *PubChainInstance) UpdatePool(poolAddr common.Address) {
	pi.poolLocker.Lock()
	defer pi.poolLocker.Unlock()
	if len(pi.lastTwoPools[1]) != 0 {
		pi.lastTwoPools[0] = pi.lastTwoPools[1]
	}
	pi.lastTwoPools[1] = poolAddr
	return
}

// GetPool get the latest two pool address
func (pi *PubChainInstance) GetPool() []common.Address {
	pi.poolLocker.RLock()
	defer pi.poolLocker.RUnlock()
	var ret []common.Address
	ret = append(ret, pi.lastTwoPools...)
	return ret
}

// GetPool get the latest two pool address
func (pi *PubChainInstance) checkToBridge(dest common.Address) bool {
	pools := pi.GetPool()
	for _, el := range pools {
		if dest.String() == el.String() {
			return true
		}
	}
	return false
}

//DeleteExpired delete the expired tx
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
