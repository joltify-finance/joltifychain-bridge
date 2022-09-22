package pubchain

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	zlog "github.com/rs/zerolog/log"
	bcommon "gitlab.com/oppy-finance/oppy-bridge/common"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/types"

	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
)

const alreadyKnown = "already known"

// ProcessInBoundERC20 process the inbound contract token top-up
func (pi *Instance) ProcessInBoundERC20(tx *ethTypes.Transaction, txInfo *Erc20TxInfo, txBlockHeight uint64) error {
	err := pi.processInboundERC20Tx(tx.Hash().Hex()[2:], txBlockHeight, txInfo.fromAddr, txInfo.tokenAddress, txInfo.Amount, txInfo.tokenAddress)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to process the inbound tx")
		return err
	}
	return nil
}

// ProcessNewBlock process the blocks received from the public pub_chain
func (pi *Instance) ProcessNewBlock(number *big.Int) error {
	block, err := pi.GetBlockByNumberWithLock(number)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to retrieve the block")
		return err
	}
	// we need to put the block height in which we find the tx
	pi.processEachBlock(block, number.Int64())
	return nil
}

func (pi *Instance) processInboundERC20Tx(txID string, txBlockHeight uint64, from types.AccAddress, to common.Address, value *big.Int, addr common.Address) error {
	// this is repeated check for tokenAddr which is cheked at function 'processEachBlock'
	tokenItem, exit := pi.TokenList.GetTokenInfoByAddress(strings.ToLower(addr.Hex()))
	if !exit {
		pi.logger.Error().Msgf("Token is not on our token list")
		return errors.New("token is not on our token list")
	}

	token := types.Coin{
		Denom:  tokenItem.Denom,
		Amount: types.NewIntFromBigInt(value),
	}

	tx := InboundTx{
		txID,
		from,
		txBlockHeight,
		token,
	}

	txIDBytes, err := hex.DecodeString(txID)
	if err != nil {
		pi.logger.Warn().Msgf("invalid tx ID %v\n", txIDBytes)
		return nil
	}

	delta := types.Precision - tokenItem.Decimals
	if delta != 0 {
		adjustedTokenAmount := bcommon.AdjustInt(tx.Token.Amount, int64(delta))
		tx.Token.Amount = adjustedTokenAmount
	}

	item := bcommon.NewAccountInboundReq(tx.Address, to, tx.Token, txIDBytes, int64(txBlockHeight))
	pi.AddItem(&item)
	return nil
}

func (pi *Instance) checkErc20(data []byte, to string) (*Erc20TxInfo, error) {
	// address toAddress, uint256 amount, address contractAddress, bytes memo

	// check it is from our smart contract
	if !strings.EqualFold(to, OppyContractAddress) {
		return nil, errors.New("not our smart contract")
	}

	if method, ok := pi.tokenAbi.Methods["oppyTransfer"]; ok {
		if len(data) < 4 {
			return nil, errors.New("invalid data")
		}
		params, err := method.Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		if len(params) != 4 {
			return nil, errors.New("invalid transfer parameter")
		}
		toAddr, ok := params[0].(common.Address)
		if !ok {
			return nil, errors.New("not valid address")
		}
		amount, ok := params[1].(*big.Int)
		if !ok {
			return nil, errors.New("not valid amount")
		}
		tokenAddress, ok := params[2].(common.Address)
		if !ok {
			return nil, errors.New("not valid address")
		}
		memo, ok := params[3].([]byte)
		if !ok {
			return nil, errors.New("not valid memo")
		}
		var memoInfo bcommon.BridgeMemo
		err = json.Unmarshal(memo, &memoInfo)
		if err != nil {
			return nil, err
		}

		fromAddr, err := types.AccAddressFromBech32(memoInfo.Dest)
		if err != nil {
			return nil, err
		}
		ret := Erc20TxInfo{
			fromAddr:     fromAddr,
			toAddr:       toAddr,
			Amount:       amount,
			tokenAddress: tokenAddress,
		}

		return &ret, nil
	}
	return nil, errors.New("invalid method for decode")
}

func (pi *Instance) processEachBlock(block *ethTypes.Block, txBlockHeight int64) {
	for _, tx := range block.Transactions() {
		if tx.To() == nil {
			continue
		}
		status, err := pi.checkEachTx(tx.Hash())
		if err != nil || status != 1 {
			continue
		}

		txInfo, err := pi.checkErc20(tx.Data(), tx.To().Hex())
		if err == nil {
			_, exit := pi.TokenList.GetTokenInfoByAddress(txInfo.tokenAddress.String())
			if !exit {
				// this indicates it is not to our smart contract
				continue
			}
			// process the public chain inbound message to the channel
			if !pi.checkToBridge(txInfo.toAddr) {
				pi.logger.Warn().Msg("the top up message is not to the bridge, ignored")
				continue
			}

			err := pi.ProcessInBoundERC20(tx, txInfo, block.NumberU64())
			if err != nil {
				zlog.Logger.Error().Err(err).Msg("fail to process the inbound contract message")
				continue
			}
			continue
		}
		if pi.checkToBridge(*tx.To()) {
			var memoInfo bcommon.BridgeMemo
			err = json.Unmarshal(tx.Data(), &memoInfo)
			if err != nil {
				pi.logger.Error().Err(err).Msgf("fail to unmarshal the memo")
				continue
			}

			fromAddr, err := types.AccAddressFromBech32(memoInfo.Dest)
			if err != nil {
				pi.logger.Error().Err(err).Msgf("fail to the acc address")
				continue
			}

			// this indicates it is a native bnb transfer
			balance, err := pi.getBalance(tx.Value())
			if err != nil {
				continue
			}
			tokenItem, exist := pi.TokenList.GetTokenInfoByAddress("native")
			if !exist {
				panic("native token is not set")
			}
			delta := types.Precision - tokenItem.Decimals
			if delta != 0 {
				adjustedTokenAmount := bcommon.AdjustInt(balance.Amount, int64(delta))
				balance.Amount = adjustedTokenAmount
			}

			item := bcommon.NewAccountInboundReq(fromAddr, *tx.To(), balance, tx.Hash().Bytes(), txBlockHeight)
			// we add to the retry pool to  sort the tx
			pi.AddItem(&item)
		}
	}
}

// UpdatePool update the tss pool address
func (pi *Instance) UpdatePool(pool *vaulttypes.PoolInfo) error {
	if pool == nil {
		return errors.New("nil pool")
	}
	poolPubKey := pool.CreatePool.PoolPubKey
	addr, err := misc.PoolPubKeyToOppyAddress(poolPubKey)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to convert the oppy address to eth address %v", poolPubKey)
		return err
	}

	ethAddr, err := misc.PoolPubKeyToEthAddress(poolPubKey)
	if err != nil {
		fmt.Printf("fail to convert the oppy address to eth address %v", poolPubKey)
		return err
	}

	pi.poolLocker.Lock()
	defer pi.poolLocker.Unlock()

	p := bcommon.PoolInfo{
		Pk:          poolPubKey,
		OppyAddress: addr,
		EthAddress:  ethAddr,
		PoolInfo:    pool,
	}

	if pi.lastTwoPools[1] != nil {
		pi.lastTwoPools[0] = pi.lastTwoPools[1]
	}
	pi.lastTwoPools[1] = &p
	return nil
}

// GetPool get the latest two pool address
func (pi *Instance) GetPool() []*bcommon.PoolInfo {
	pi.poolLocker.RLock()
	defer pi.poolLocker.RUnlock()
	var ret []*bcommon.PoolInfo
	ret = append(ret, pi.lastTwoPools...)
	return ret
}

// GetPool get the latest two pool address
func (pi *Instance) checkToBridge(dest common.Address) bool {
	pools := pi.GetPool()
	for _, el := range pools {
		if el != nil && dest.String() == el.EthAddress.String() {
			return true
		}
	}
	return false
}
