package pubchain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"math"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/generated"

	"github.com/containerd/containerd/pkg/atomic"
	"github.com/ethereum/go-ethereum/ethclient"
	zlog "github.com/rs/zerolog/log"

	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
)

// MoveFound moves the fund for the public chain
// our strategy is we need to run move fund at least twice to ensure the account is empty, even if
// we move the fund success this round, we still need to run it again to 100% ensure the old pool is empty
func (pi *Instance) MoveFound(height int64, chainInfo *ChainInfo, previousPool *bcommon.PoolInfo, ethClient *ethclient.Client) bool {
	// we get the latest pool address and move funds to the latest pool
	currentPool := pi.GetPool()
	emptyERC20Tokens := atomic.NewBool(true)

	// if we restart the bridge, pubchain go routine may run before joltify go routine which acquire the pool info
	if currentPool[1] == nil {
		zlog.Warn().Msgf("the current pool has not been set, move fund can not start")
		return false
	}

	// movefund according to the history tokenlist
	existedTokenAddresses := pi.TokenList.GetAllExistedTokenAddresses(chainInfo.ChainType)
	sort.Strings(existedTokenAddresses)
	var needToMove []string
	waitForFinish := &sync.WaitGroup{}
	for _, tokenAddr := range existedTokenAddresses {
		if tokenAddr == "native" {
			continue
		}
		needMove, err := pi.needToMoveFund(tokenAddr, previousPool.EthAddress, ethClient)
		if err != nil {
			pi.AddMoveFundItem(previousPool, height+movefundretrygap, chainInfo.ChainType)
			return false
		}
		if needMove {
			needToMove = append(needToMove, tokenAddr)
		}
	}

	if len(needToMove) == 0 {
		_, isEmpty, err := pi.doMoveBNBFunds(chainInfo, previousPool, currentPool[1].EthAddress)
		if isEmpty {
			return true
		}
		if err != nil {
			pi.logger.Error().Err(err).Msgf("fail to move the native bnb")
		}
		pi.AddMoveFundItem(previousPool, height+movefundretrygap, chainInfo.ChainType)
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	nonce, err := chainInfo.getPendingNonceWithLock(ctx, previousPool.EthAddress)
	if err != nil {
		pi.AddMoveFundItem(previousPool, height+movefundretrygap, chainInfo.ChainType)
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
			ethClientLocal, err := ethclient.Dial(chainInfo.WsAddr)
			if err != nil {
				pi.AddMoveFundItem(previousPool, height, chainInfo.ChainType)
				zlog.Logger.Error().Err(err).Msg("fail to dial the websocket")
			}

			myNonce := nonce + uint64(index)
			tokenIsEmpty, err := pi.doMoveTokenFunds(chainInfo, index, myNonce, previousPool, currentPool[1].EthAddress, thisTokenAddr, ethClientLocal, tssReqChan, tssRespChan)
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
		latest, err := chainInfo.GetBlockByNumberWithLock(nil)
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
		pi.AddMoveFundItem(previousPool, height+movefundretrygap, chainInfo.ChainType)
		return false
	}
	bnbIsMoved, isEmpty, errMoveBnb := pi.doMoveBNBFunds(chainInfo, previousPool, currentPool[1].EthAddress)
	if isEmpty {
		return true
	}
	pi.AddMoveFundItem(previousPool, height+movefundretrygap, chainInfo.ChainType)
	if err != nil || !bnbIsMoved {
		zlog.Log().Err(errMoveBnb).Msgf("fail to move the fund from %v to %v for bnb", previousPool.EthAddress.String(), currentPool[1].EthAddress.String())
		return false
	}
	return true
}

func (pi *Instance) moveERC20Token(chainInfo *ChainInfo, index int, nonce uint64, sender, receiver common.Address, balance *big.Int, tokenAddr string, tssReqChan chan *TssReq, tssRespChan chan map[string][]byte) (common.Hash, error) {
	txHash, err := pi.SendTokenBatch(chainInfo, index, sender, receiver, balance, big.NewInt(int64(nonce)), tokenAddr, tssReqChan, tssRespChan)
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

func (pi *Instance) needToMoveFund(tokenAddr string, poolAddr common.Address, ethClient *ethclient.Client) (bool, error) {
	tokenInstance, err := generated.NewToken(common.HexToAddress(tokenAddr), ethClient)
	if err != nil {
		return true, err
	}
	balance, err := tokenInstance.BalanceOf(&bind.CallOpts{}, poolAddr)
	if err != nil {
		return true, err
	}

	if balance.Cmp(big.NewInt(0)) == 1 {
		tick := html.UnescapeString("&#" + "9193" + ";")
		pi.logger.Info().Msgf(" %v we move fund %v %v from %v ", tick, tokenAddr, balance, poolAddr)
		return true, nil
	}
	return false, nil
}

func (pi *Instance) doMoveTokenFunds(chainInfo *ChainInfo, index int, nonce uint64, previousPool *bcommon.PoolInfo, receiver common.Address, tokenAddr string, ethClient *ethclient.Client, tssReqChan chan *TssReq, tssRespChan chan map[string][]byte) (bool, error) {
	tokenInstance, err := generated.NewToken(common.HexToAddress(tokenAddr), ethClient)
	if err != nil {
		return false, err
	}
	balance, err := tokenInstance.BalanceOf(&bind.CallOpts{}, previousPool.EthAddress)
	if err != nil {
		return false, err
	}

	if balance.Cmp(big.NewInt(0)) != 1 {
		pi.logger.Warn().Msg("0 ERC20 balance do not need to move")
		return true, nil
	}

	erc20TxHash, err := pi.moveERC20Token(chainInfo, index, nonce, previousPool.EthAddress, receiver, balance, tokenAddr, tssReqChan, tssRespChan)
	// if we fail erc20 token transfer, we should not transfer the bnb otherwise,we do not have enough fee to pay retry
	if err != nil && err.Error() != "already passed the seq" {
		return false, errors.New("fail to transfer erc20 token")
	}

	err1 := chainInfo.CheckTxStatus(erc20TxHash.Hex())
	if err1 != nil {
		return false, err1
	}

	nowBalance, err2 := tokenInstance.BalanceOf(&bind.CallOpts{}, previousPool.EthAddress)
	if err2 == nil && nowBalance.Cmp(big.NewInt(0)) != 1 {
		tick := html.UnescapeString("&#" + "127974" + ";")
		zlog.Logger.Info().Msgf(" %v(%v) we have moved the erc20 %v with hash %v", tick, chainInfo.ChainType, balance.String(), erc20TxHash)
		return true, nil
	}

	return false, errors.New("we failed to move fund for this token")
}

func (pi *Instance) doMoveBNBFunds(chainInfo *ChainInfo, previousPool *bcommon.PoolInfo, receiver common.Address) (bool, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.QueryTimeOut)
	defer cancel()
	balanceBnB, err := chainInfo.getBalanceWithLock(ctx, previousPool.EthAddress)
	if err != nil {
		return false, false, err
	}

	fee, _, _, err := chainInfo.GetFeeLimitWithLock()
	if err != nil {
		return false, false, err
	}

	// this indicates we have the leftover that smaller than the fee
	if balanceBnB.Cmp(fee) != 1 {
		return true, true, nil
	}

	tick := html.UnescapeString("&#" + "9193" + ";")
	pi.logger.Info().Msgf(" %v we move fund %v:%v from %v to %v", tick, chainInfo.ChainType, balanceBnB, previousPool.EthAddress.String(), receiver.String())

	// we move the bnb
	nonce, err := chainInfo.getPendingNonceWithLock(ctx, previousPool.EthAddress)
	if err != nil {
		return false, false, err
	}

	bnbTxHash, emptyAccount, err := pi.SendNativeTokenForMoveFund(chainInfo, previousPool.Pk, previousPool.EthAddress, receiver, balanceBnB, new(big.Int).SetUint64(nonce))
	// bnbTxHash, err = pi.moveBnb(previousPool.Pk, receiver, balanceBnB, nonce, blockHeight)
	if err != nil {
		if err.Error() == "already passed the seq" {
			ctx2, cancel2 := context.WithTimeout(context.Background(), config.QueryTimeOut)
			defer cancel2()
			nowBalanceBnB, err := chainInfo.getBalanceWithLock(ctx2, previousPool.EthAddress)
			if err != nil {
				return false, false, err
			}

			// this indicates we have the leftover that smaller than the fee
			if nowBalanceBnB.Cmp(fee) != 1 {
				return true, true, nil
			}
		}
		return false, false, err
	}
	if emptyAccount {
		zlog.Logger.Info().Msgf("this is the empty account to move fund")
		return true, true, nil
	}

	errCheck := chainInfo.CheckTxStatus(bnbTxHash.Hex())
	if errCheck != nil {
		return false, false, errCheck
	}
	tick = html.UnescapeString("&#" + "127974" + ";")
	zlog.Logger.Info().Msgf(" %v we have moved the fund in the public chain (%v): %v with hash %v", tick, chainInfo.ChainType, balanceBnB.String(), bnbTxHash)

	ctx2, cancel2 := context.WithTimeout(context.Background(), config.QueryTimeOut)
	defer cancel2()
	nowBalanceBnB, err := chainInfo.getBalanceWithLock(ctx2, previousPool.EthAddress)
	if err != nil {
		return false, false, err
	}

	// this indicate we have the leftover that smaller than the fee
	if nowBalanceBnB.Cmp(fee) != 1 {
		return true, true, nil
	}
	return true, false, nil
}

func (pi *Instance) AddMoveFundItem(pool *bcommon.PoolInfo, height int64, chainType string) {
	item := bcommon.MoveFundItem{PoolInfo: pool, ChainType: chainType, Height: height}
	index := fmt.Sprintf("%v:%v", height, chainType)
	pi.moveFundReq.Store(index, &item)
}

func (pi *Instance) ExportMoveFundItems() []*bcommon.MoveFundItem {
	var data []*bcommon.MoveFundItem
	pi.moveFundReq.Range(func(key, value any) bool {
		exported := value.(*bcommon.MoveFundItem)
		data = append(data, exported)
		return true
	})
	return data
}

// PopMoveFundItemAfterBlock pop up the item after the given block duration
func (pi *Instance) PopMoveFundItemAfterBlock(currentBlockHeight int64, chainType string) (*bcommon.MoveFundItem, int64) {
	min := int64(math.MaxInt64)
	pi.moveFundReq.Range(func(key, value interface{}) bool {
		val := value.(*bcommon.MoveFundItem)
		h := val.Height
		if h <= min && val.ChainType == chainType {
			min = h
		}
		return true
	})

	if min < math.MaxInt64 && (currentBlockHeight-min > config.MINCHECKBLOCKGAP) {
		index := fmt.Sprintf("%v:%v", min, chainType)
		item, _ := pi.moveFundReq.LoadAndDelete(index)
		return item.(*bcommon.MoveFundItem), min
	}
	return nil, 0
}
