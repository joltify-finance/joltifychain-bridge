package pubchain

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
	zlog "github.com/rs/zerolog/log"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

// ProcessInBoundERC20 process the inbound contract token top-up
func (pi *PubChainInstance) ProcessInBoundERC20(tx *ethTypes.Transaction, tokenAddr, transferTo common.Address, amount *big.Int, blockHeight uint64) error {
	v, r, s := tx.RawSignatureValues()
	signer := ethTypes.LatestSignerForChainID(tx.ChainId())
	plainV := misc.RecoverRecID(tx.ChainId().Uint64(), v)
	sigBytes := misc.MakeSignature(r, s, plainV)

	sigPublicKey, err := crypto.Ecrecover(signer.Hash(tx).Bytes(), sigBytes)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to recover the public key")
		return err
	}

	transferFrom, err := misc.EthSignPubKeyToJoltAddr(sigPublicKey)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to recover the joltify Address")
		return err
	}

	err = pi.processInboundTx(tx.Hash().Hex()[2:], blockHeight, transferFrom, transferTo, amount, tokenAddr)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to process the inbound tx")
		return err
	}
	return nil
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
func (pi *PubChainInstance) updateInboundTx(txID string, amount *big.Int, blockNum uint64) *inboundTx {
	data, ok := pi.pendingInbounds.Load(txID)
	if !ok {
		pi.logger.Warn().Msgf("inbound fail to get the stored tx from pool with %v\n", pi.pendingInbounds)
		inBnB := inboundTxBnb{
			blockHeight: blockNum,
			txID:        txID,
			fee:         sdk.NewCoin(config.InBoundDenomFee, sdk.NewIntFromBigInt(amount)),
		}
		pi.pendingInboundsBnB.Store(txID, &inBnB)
		return nil
	}

	thisAccount := data.(*inboundTx)
	thisAccount.fee.Amount = thisAccount.fee.Amount.Add(types.NewIntFromBigInt(amount))
	err := thisAccount.Verify()
	if err != nil {
		pi.pendingInbounds.Store(txID, thisAccount)
		pi.logger.Warn().Msgf("the account cannot be processed on joltify pub_chain this round with err %v\n", err)
		return nil
	}
	// since this tx is processed,we do not need to store it any longer
	pi.pendingInbounds.Delete(txID)
	return thisAccount
}

func (pi *PubChainInstance) processInboundTx(txID string, blockHeight uint64, from types.AccAddress, to common.Address, value *big.Int, addr common.Address) error {
	_, ok := pi.pendingInbounds.Load(txID)
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

	inTxBnB, ok := pi.pendingInboundsBnB.LoadAndDelete(txID)
	if !ok {
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
		pi.pendingInbounds.Store(txID, &tx)
		return nil
	}
	fee := inTxBnB.(*inboundTxBnb).fee
	tx := inboundTx{
		from,
		blockHeight,
		token,
		fee,
	}
	err := tx.Verify()
	if err != nil {
		pi.pendingInbounds.Store(txID, &tx)
		pi.logger.Warn().Msgf("the account cannot be processed on joltify pub_chain this round with err %v\n", err)
		return nil
	}
	txIDBytes, err := hex.DecodeString(txID)
	if err != nil {
		pi.logger.Warn().Msgf("invalid tx ID %v\n", txIDBytes)
		return nil
	}
	item := NewAccountInboundReq(tx.address, to, tx.token, txIDBytes, blockHeight)
	pi.InboundReqChan <- &item
	return nil
}

func (pi *PubChainInstance) checkErc20(data []byte) (common.Address, *big.Int, error) {
	if method, ok := pi.tokenAbi.Methods["transfer"]; ok {
		if len(data) < 4 {
			return common.Address{}, nil, errors.New("invalid data")
		}
		params, err := method.Inputs.Unpack(data[4:])
		if err != nil {
			return common.Address{}, nil, err
		}
		if len(params) != 2 {
			return common.Address{}, nil, errors.New("invalid transfer parameter")
		}
		toAddr, ok := params[0].(common.Address)
		if !ok {
			return common.Address{}, nil, errors.New("not valid address")
		}
		amount, ok := params[1].(*big.Int)
		if !ok {
			return common.Address{}, nil, errors.New("not valid amount")
		}
		return toAddr, amount, nil
	}
	return common.Address{}, nil, errors.New("invalid method for decode")
}

// fixme we need to check timeout to remove the pending transactions
func (pi *PubChainInstance) processEachBlock(block *ethTypes.Block) {
	for _, tx := range block.Transactions() {
		if tx.To() == nil || tx.Value() == nil {
			continue
		}

		toAddr, amount, err := pi.checkErc20(tx.Data())
		if err == nil {
			if tx.To().Hex() != pi.tokenAddr {
				// this indicates it is not to our smart contract
				continue
			}
			// process the public chain inbound message to the channel
			if !pi.checkToBridge(toAddr) {
				pi.logger.Warn().Msg("the top up message is not to the bridge, ignored")
				continue
			}
			err := pi.ProcessInBoundERC20(tx, *tx.To(), toAddr, amount, block.NumberU64())
			if err != nil {
				zlog.Logger.Error().Err(err).Msg("fail to process the inbound contract message")
				continue
			}
		}

		if pi.checkToBridge(*tx.To()) {
			if tx.Data() == nil {
				pi.logger.Warn().Msgf("we have received unknown fund")
				continue
			}

			payTxID := tx.Data()
			account := pi.updateInboundTx(hex.EncodeToString(payTxID), tx.Value(), block.NumberU64())
			if account != nil {
				item := NewAccountInboundReq(account.address, *tx.To(), account.token, payTxID, block.Number().Uint64())
				pi.InboundReqChan <- &item
			}
		}
	}
}

// UpdatePool update the tss pool address
func (pi *PubChainInstance) UpdatePool(pool *vaulttypes.PoolInfo) error {
	poolPubKey := pool.CreatePool.PoolPubKey
	addr, err := misc.PoolPubKeyToJoltAddress(poolPubKey)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to convert the jolt addres to eth address %v", poolPubKey)
		return err
	}

	ethAddr, err := misc.PoolPubKeyToEthAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the jolt address to eth address %v", poolPubKey)
		return err
	}

	pi.poolLocker.Lock()
	defer pi.poolLocker.Unlock()

	p := bcommon.PoolInfo{
		Pk:             poolPubKey,
		JoltifyAddress: addr,
		EthAddress:     ethAddr,
		PoolInfo:       pool,
	}

	if pi.lastTwoPools[1] != nil {
		pi.lastTwoPools[0] = pi.lastTwoPools[1]
	}
	pi.lastTwoPools[1] = &p
	return nil
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
	var expiredTx []string
	var expiredTxBnb []string
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		el := value.(*inboundTx)
		if currentHeight-el.blockHeight > config.TxTimeout {
			expiredTx = append(expiredTx, key.(string))
		}
		return true
	})

	for _, el := range expiredTx {
		pi.logger.Warn().Msgf("we delete the expired tx %s", el)
		pi.pendingInbounds.Delete(el)
	}

	pi.pendingInboundsBnB.Range(func(key, value interface{}) bool {
		el := value.(*inboundTxBnb)
		if currentHeight-el.blockHeight > config.TxTimeout {
			expiredTxBnb = append(expiredTxBnb, key.(string))
		}
		return true
	})

	for _, el := range expiredTxBnb {
		pi.logger.Warn().Msgf("we delete the expired tx %s in inbound bnb", el)
		pi.pendingInboundsBnB.Delete(el)
	}
}

// Verify is the function  to verify the correctness of the account on joltify_bridge
func (a *inboundTx) Verify() error {
	if a.fee.Denom != config.InBoundDenomFee {
		return fmt.Errorf("invalid inbound fee denom with fee demo : %v and want %v", a.fee.Denom, config.InBoundDenom)
	}
	amount, err := sdk.NewDecFromStr(config.InBoundFeeMin)
	if err != nil {
		return errors.New("invalid minimal inbound fee")
	}
	if a.fee.Amount.LT(sdk.NewIntFromBigInt(amount.BigInt())) {
		return errors.New("the fee is not enough")
	}
	return nil
}
