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
func (ci *PubChainInstance) ProcessInBound(transfer *TokenTransfer) error {
	if transfer.Raw.Removed {
		return errors.New("the tx is the revert tx")
	}
	tokenAddr := transfer.Raw.Address
	err := ci.addBridgeTx(transfer.Raw.TxHash.Hex()[2:], transfer.From, transfer.Value, tokenAddr, inBound)
	return err
}

// ProcessNewBlock process the blocks received from the public pub_chain
func (ci *PubChainInstance) ProcessNewBlock(number *big.Int) error {
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	block, err := ci.EthClient.BlockByNumber(ctx, number)
	if err != nil {
		ci.logger.Error().Err(err).Msg("fail to retrieve the block")
		return err
	}
	ci.processEachBlock(block)
	return nil
}

// updateBridgeTx update the top-up token with fee
func (ci *PubChainInstance) updateBridgeTx(txID string, amount *big.Int, direction direction) *bridgeTx {
	ci.pendingAccountLocker.Lock()
	defer ci.pendingAccountLocker.Unlock()
	thisAccount, ok := ci.pendingAccounts[txID]
	if !ok {
		ci.logger.Warn().Msg("fail to get the stored tx from pool")
		return nil
	}
	if thisAccount.direction != direction {
		ci.logger.Warn().Msg("the tx direction is not consistent")
		return nil
	}
	thisAccount.fee.Amount = thisAccount.fee.Amount.Add(types.NewIntFromBigInt(amount))
	err := thisAccount.Verify()
	if err != nil {
		ci.pendingAccounts[txID] = thisAccount
		ci.logger.Warn().Msgf("the account cannot be processed on joltify pub_chain this round")
		return nil
	}
	// since this tx is processed,we do not need to store it any longer
	delete(ci.pendingAccounts, txID)
	return thisAccount
}

func (ci *PubChainInstance) addBridgeTx(txID string, from common.Address, value *big.Int, addr common.Address, direction direction) error {
	ci.pendingAccountLocker.Lock()
	defer ci.pendingAccountLocker.Unlock()
	_, ok := ci.pendingAccounts[txID]
	if ok {
		ci.logger.Error().Msgf("the tx already exist!!")
		return errors.New("tx existed")
	}

	if addr.String() != iNBoundToken {
		ci.logger.Error().Msgf("incorrect top up token")
		return errors.New("incorrect top up token")
	}

	token := types.Coin{
		Denom:  iNBoundTokenSymbol,
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
	ci.logger.Info().Msgf("we add the tokens %v", acc.token.String())
	ci.pendingAccounts[txID] = &acc
	return nil
}

//fixme we need to check timeout to remove the pending transactions
func (ci *PubChainInstance) processEachBlock(block *ethTypes.Block) {
	for _, tx := range block.Transactions() {
		if tx.To() == nil || tx.Value() == nil {
			continue
		}
		if ci.checkToBridge(*tx.To()) {
			if tx.Data() == nil {
				ci.logger.Warn().Msgf("we have received unknown fund")
				continue
			}
			payTxID := tx.Data()
			account := ci.updateBridgeTx(hex.EncodeToString(payTxID), tx.Value(), inBound)
			if account != nil {
				item := newAccountInboundReq(account.address, *tx.To(), account.token)
				ci.AccountInboundReqChan <- &item
				//fmt.Printf("BridgeTx %s is ready to send %v\n tokens to joltify Chain!!!", account.address, account.token.String())
			}
		}
	}
}

// UpdatePool update the tss pool address
func (ci *PubChainInstance) UpdatePool(poolAddr common.Address) {
	ci.poolLocker.Lock()
	defer ci.poolLocker.Unlock()
	if len(ci.lastTwoPools[1]) != 0 {
		ci.lastTwoPools[0] = ci.lastTwoPools[1]
	}
	ci.lastTwoPools[1] = poolAddr
	return
}

// GetPool get the latest two pool address
func (ci *PubChainInstance) GetPool() []common.Address {
	ci.poolLocker.RLock()
	defer ci.poolLocker.RUnlock()
	var ret []common.Address
	ret = append(ret, ci.lastTwoPools...)
	return ret
}

// GetPool get the latest two pool address
func (ci *PubChainInstance) checkToBridge(dest common.Address) bool {
	pools := ci.GetPool()
	for _, el := range pools {
		if dest.String() == el.String() {
			return true
		}
	}
	return false
}
