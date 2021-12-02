package pubchain

import (
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"time"

	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
)

const chainQueryTimeout = time.Second * 5

// ProcessInBound process the inbound contract token top-up
func (pi *PubChainInstance) ProcessInBound(transfer *TokenTransfer) error {
	if transfer.Raw.Removed {
		return errors.New("the tx is the revert tx")
	}
	tokenAddr := transfer.Raw.Address
	err := pi.addBridgeTx(transfer.Raw.TxHash.Hex()[2:], transfer.From, transfer.Value, tokenAddr, inBound)
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

// updateBridgeTx update the top-up token with fee
func (pi *PubChainInstance) updateBridgeTx(txID string, amount *big.Int, direction direction) *bridgeTx {
	pi.pendingAccountLocker.Lock()
	defer pi.pendingAccountLocker.Unlock()
	thisAccount, ok := pi.pendingAccounts[txID]
	if !ok {
		pi.logger.Warn().Msgf("fail to get the stored tx from pool with %v\n", pi.pendingAccounts)
		return nil
	}
	if thisAccount.direction != direction {
		pi.logger.Warn().Msg("the tx direction is not consistent")
		return nil
	}
	thisAccount.fee.Amount = thisAccount.fee.Amount.Add(types.NewIntFromBigInt(amount))
	err := thisAccount.Verify()

	if err != nil {
		pi.pendingAccounts[txID] = thisAccount
		pi.logger.Warn().Msgf("the account cannot be processed on joltify pub_chain this round")
		return nil
	}
	// since this tx is processed,we do not need to store it any longer
	delete(pi.pendingAccounts, txID)
	return thisAccount
}

func (pi *PubChainInstance) addBridgeTx(txID string, from common.Address, value *big.Int, addr common.Address, direction direction) error {
	pi.pendingAccountLocker.Lock()
	defer pi.pendingAccountLocker.Unlock()
	_, ok := pi.pendingAccounts[txID]
	if ok {
		pi.logger.Error().Msgf("the tx already exist!!")
		return errors.New("tx existed")
	}

	if addr.String() != pi.tokenAddr {
		pi.logger.Error().Msgf("incorrect top up token")
		return errors.New("incorrect top up token")
	}

	token := types.Coin{
		Denom:  iNBoundDenom,
		Amount: types.NewIntFromBigInt(value),
	}
	fee := types.Coin{
		Denom:  inBoundDenom,
		Amount: types.NewInt(0),
	}

	acc := bridgeTx{
		from,
		direction,
		time.Now(),
		token,
		fee,
	}
	pi.logger.Info().Msgf("we add the tokens tx(%v):%v", txID, acc.token.String())
	pi.pendingAccounts[txID] = &acc
	pi.logger.Info().Msgf("after add the tokens %v", pi.pendingAccounts)
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
			account := pi.updateBridgeTx(hex.EncodeToString(payTxID), tx.Value(), inBound)
			if account != nil {
				item := newAccountInboundReq(account.address, *tx.To(), account.token, block.Number().Int64())
				pi.AccountInboundReqChan <- &item
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
