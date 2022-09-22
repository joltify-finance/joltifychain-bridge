package pubchain

import (
	"bytes"
	"context"
	"errors"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/generated"
	"html"
	"math"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/containerd/containerd/pkg/atomic"
	"github.com/ethereum/go-ethereum/ethclient"
	zlog "github.com/rs/zerolog/log"

	bcommon "gitlab.com/oppy-finance/oppy-bridge/common"
)

// MoveFound moves the fund for the public chain
// our strategy is we need to run move fund at least twice to ensure the account is empty, even if
// we move the fund success this round, we still need to run it again to 100% ensure the old pool is empty
func (pi *Instance) MoveFound(height int64, previousPool *bcommon.PoolInfo, ethClient *ethclient.Client) bool {
	// we get the latest pool address and move funds to the latest pool
	currentPool := pi.GetPool()
	emptyERC20Tokens := atomic.NewBool(true)

	// if we restart the bridge, pubchain go routine may run before oppy go routine which acquire the pool info
	if currentPool[1] == nil {
		zlog.Warn().Msgf("the current pool has not been set, move fund can not start")
		return false
	}

	// movefund according to the history tokenlist
	existedTokenAddresses := pi.TokenList.GetAllExistedTokenAddresses()
	sort.Strings(existedTokenAddresses)

	var needToMove []string
	waitForFinish := &sync.WaitGroup{}
	for _, tokenAddr := range existedTokenAddresses {
		if tokenAddr == "native" {
			continue
		}
		needToMove = append(needToMove, tokenAddr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	nonce, err := pi.getPendingNonceWithLock(ctx, previousPool.EthAddress)
	if err != nil {
		pi.AddMoveFundItem(previousPool, height+movefundretrygap)
		return false
	}
	tssReqChan := make(chan *TssReq, len(needToMove))
	defer close(tssReqChan)
	bc := NewBroadcaster()
	for i, tokenAddr := range needToMove {
		waitForFinish.Add(1)
		go func(index int, thisTokenAddr string) {
			defer waitForFinish.Done()
			tssRespChan, err := bc.Subscribe(int64(index))
			if err != nil {
				panic("should not been subscribed!!")
			}
			defer bc.Unsubscribe(int64(index))
			myNonce := nonce + uint64(index)
			tokenIsEmpty, err := pi.doMoveTokenFunds(index, myNonce, previousPool, currentPool[1].EthAddress, thisTokenAddr, ethClient, tssReqChan, tssRespChan)
			tssReqChan <- &TssReq{Index: index, Data: []byte("done")}
			// once there exists one token in the current pool, then we need to addMoveFundItem
			if err != nil {
				zlog.Log().Err(err).Msgf("fail to move the fund from %v to %v for token %v", previousPool.EthAddress.String(), currentPool[1].EthAddress.String(), thisTokenAddr)
				emptyERC20Tokens.Unset()
				return
			}

			// once there exists non-empty token in the pool account, we have to addMoveFundItem
			if !tokenIsEmpty {
				emptyERC20Tokens.Unset()
				return
			}
		}(i, tokenAddr)
	}
	waitForFinish.Add(1)
	go func() {
		defer waitForFinish.Done()
		var allsignMSgs [][]byte
		received := make(map[int][]byte)
		collected := false
		for {
			msg := <-tssReqChan
			received[msg.Index] = msg.Data
			if len(received) >= len(needToMove) {
				collected = true
			}
			if collected {
				break
			}
		}
		for _, val := range received {
			if bytes.Equal([]byte("none"), val) {
				continue
			}
			allsignMSgs = append(allsignMSgs, val)
		}
		var err error
		latest, err := pi.GetBlockByNumberWithLock(nil)
		if err != nil {
			zlog.Logger.Error().Err(err).Msgf("fail to get the latest height")
			bc.Broadcast(nil)
			return
		}
		blockHeight := int64(latest.NumberU64()) / ROUNDBLOCK
		signature, err := pi.TssSignBatch(allsignMSgs, previousPool.Pk, blockHeight)
		if err != nil {
			zlog.Logger.Error().Err(err).Msgf("fail to get the latest height")
			bc.Broadcast(nil)
			return
		}
		bc.Broadcast(signature)
	}()

	waitForFinish.Wait()

	if !emptyERC20Tokens.IsSet() {
		// we add this account to "retry" to ensure it is the empty account in the next balance check
		pi.AddMoveFundItem(previousPool, height+movefundretrygap)
		return false
	}
	bnbIsMoved, isEmpty, err := pi.doMoveBNBFunds(previousPool, currentPool[1].EthAddress)
	if isEmpty {
		return true
	}
	pi.AddMoveFundItem(previousPool, height+movefundretrygap)
	if err != nil || !bnbIsMoved {
		zlog.Log().Err(err).Msgf("fail to move the fund from %v to %v for bnb", previousPool.EthAddress.String(), currentPool[1].EthAddress.String())
		return false
	}
	return true
}

func (pi *Instance) moveERC20Token(index int, nonce uint64, sender, receiver common.Address, balance *big.Int, tokenAddr string, tssReqChan chan *TssReq, tssRespChan chan map[string][]byte) (common.Hash, error) {
	txHash, err := pi.SendTokenBatch(index, sender, receiver, balance, big.NewInt(int64(nonce)), tokenAddr, tssReqChan, tssRespChan)
	if err != nil {
		if err.Error() == alreadyKnown {
			pi.logger.Warn().Msgf("the tx has been submitted by others")
			return txHash, nil
		}
		pi.logger.Error().Err(err).Msgf("fail to send the token with err %v for amount %v ", err, balance)
		return txHash, err
	}
	return txHash, nil
}

func (pi *Instance) doMoveTokenFunds(index int, nonce uint64, previousPool *bcommon.PoolInfo, receiver common.Address, tokenAddr string, ethClient *ethclient.Client, tssReqChan chan *TssReq, tssRespChan chan map[string][]byte) (bool, error) {
	tokenInstance, err := generated.NewToken(common.HexToAddress(tokenAddr), ethClient)
	if err != nil {
		return false, err
	}
	balance, err := tokenInstance.BalanceOf(&bind.CallOpts{}, previousPool.EthAddress)
	if err != nil {
		return false, err
	}

	tick := html.UnescapeString("&#" + "9193" + ";")
	pi.logger.Info().Msgf(" %v we move fund %v %v from %v to %v", tick, tokenAddr, balance, previousPool.EthAddress.String(), receiver.String())

	if balance.Cmp(big.NewInt(0)) == 0 {
		return true, nil
	}

	if balance.Cmp(big.NewInt(0)) == 1 {
		erc20TxHash, err := pi.moveERC20Token(index, nonce, previousPool.EthAddress, receiver, balance, tokenAddr, tssReqChan, tssRespChan)
		// if we fail erc20 token transfer, we should not transfer the bnb otherwise,we do not have enough fee to pay retry
		if err != nil {
			return false, errors.New("fail to transfer erc20 token")
		}

		err1 := pi.CheckTxStatus(erc20TxHash.Hex())
		if err1 != nil {
			return false, err1
		}

		nowBalance, err2 := tokenInstance.BalanceOf(&bind.CallOpts{}, previousPool.EthAddress)
		if err2 == nil && nowBalance.Cmp(big.NewInt(0)) == 0 {
			tick = html.UnescapeString("&#" + "127974" + ";")
			zlog.Logger.Info().Msgf(" %v we have moved the erc20 %v with hash %v", tick, balance.String(), erc20TxHash)
			return true, nil
		}
		return false, nil
	}
	pi.logger.Warn().Msg("0 ERC20 balance do not need to move")

	return false, errors.New("we failed to move fund for this token")
}

func (pi *Instance) doMoveBNBFunds(previousPool *bcommon.PoolInfo, receiver common.Address) (bool, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.QueryTimeOut)
	defer cancel()
	balanceBnB, err := pi.getBalanceWithLock(ctx, previousPool.EthAddress)
	if err != nil {
		return false, false, err
	}

	tick := html.UnescapeString("&#" + "9193" + ";")
	pi.logger.Info().Msgf(" %v we move fund bnb:%v from %v to %v", tick, balanceBnB, previousPool.EthAddress.String(), receiver.String())

	// we move the bnb
	nonce, err := pi.getPendingNonceWithLock(ctx, previousPool.EthAddress)
	if err != nil {
		return false, false, err
	}

	bnbTxHash, emptyAccount, err := pi.SendNativeTokenForMoveFund(previousPool.Pk, previousPool.EthAddress, receiver, balanceBnB, new(big.Int).SetUint64(nonce))
	// bnbTxHash, err = pi.moveBnb(previousPool.Pk, receiver, balanceBnB, nonce, blockHeight)
	if err != nil {
		return false, false, err
	}
	if emptyAccount {
		zlog.Logger.Info().Msgf("this is the empty account to move fund")
		return true, true, nil
	}

	errCheck := pi.CheckTxStatus(bnbTxHash.Hex())
	if errCheck != nil {
		return false, false, errCheck
	}
	tick = html.UnescapeString("&#" + "127974" + ";")
	zlog.Logger.Info().Msgf(" %v we have moved the fund in the publicchain (BNB): %v with hash %v", tick, balanceBnB.String(), bnbTxHash)

	nowBalanceBnB, err := pi.getBalanceWithLock(ctx, previousPool.EthAddress)
	if err != nil {
		return false, false, nil
	}

	_, price, _, gas, err := pi.GetFeeLimitWithLock()
	if err != nil {
		return false, false, err
	}

	adjGas := int64(float32(gas) * config.MoveFundPubChainGASFEERATIO)
	fee := new(big.Int).Mul(price, big.NewInt(adjGas))

	// this indicate we have the leftover that smaller than the fee
	if nowBalanceBnB.Cmp(fee) != 1 {
		return true, true, nil
	}

	return true, false, nil
}

func (pi *Instance) AddMoveFundItem(pool *bcommon.PoolInfo, height int64) {
	pi.moveFundReq.Store(height, pool)
}

func (pi *Instance) ExportMoveFundItems() []*bcommon.PoolInfo {
	var data []*bcommon.PoolInfo
	pi.moveFundReq.Range(func(key, value any) bool {
		exported := value.(*bcommon.PoolInfo)
		exported.Height = key.(int64)
		data = append(data, exported)
		return true
	})
	return data
}

// PopMoveFundItemAfterBlock pop up the item after the given block duration
func (pi *Instance) PopMoveFundItemAfterBlock(currentBlockHeight int64) (*bcommon.PoolInfo, int64) {
	min := int64(math.MaxInt64)
	pi.moveFundReq.Range(func(key, value interface{}) bool {
		h := key.(int64)
		if h <= min {
			min = h
		}
		return true
	})
	if min < math.MaxInt64 && (currentBlockHeight-min > config.MINCHECKBLOCKGAP) {
		item, _ := pi.moveFundReq.LoadAndDelete(min)
		return item.(*bcommon.PoolInfo), min
	}
	return nil, 0
}
