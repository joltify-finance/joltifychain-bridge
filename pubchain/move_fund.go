package pubchain

import (
	"bytes"
	"sort"
	"sync"

	"github.com/containerd/containerd/pkg/atomic"
	"github.com/ethereum/go-ethereum/ethclient"
	zlog "github.com/rs/zerolog/log"

	bcommon "gitlab.com/oppy-finance/oppy-bridge/common"
)

// MoveFound moves the fund for the public chain
// our strategy is we need to run move fund at least twice to ensure the account is empty, even if
// we move the fund success this round, we still need to run it again to 100% ensure the old pool is empty
func (pi *Instance) MoveFound(wg *sync.WaitGroup, height int64, previousPool *bcommon.PoolInfo, ethClient *ethclient.Client) bool {
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
			tokenIsEmpty, err := pi.doMoveTokenFunds(index, previousPool, currentPool[1].EthAddress, thisTokenAddr, ethClient, tssReqChan, tssRespChan)
			tssReqChan <- &TssReq{Index: index, Data: []byte("done")}
			// once there exists one token in the current pool, then we need to addMoveFundItem
			if err != nil {
				zlog.Log().Err(err).Msgf("fail to move the fund from %v to %v for token %v", previousPool.EthAddress.String(), currentPool[1].EthAddress.String(), tokenAddr)
				emptyERC20Tokens.Set()
				return
			}

			// once there exists non-empty token in the pool account, we have to addMoveFundItem
			if !tokenIsEmpty {
				emptyERC20Tokens.Set()
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
			select {
			case msg := <-tssReqChan:
				received[msg.Index] = msg.Data
				if len(received) >= len(needToMove) {
					collected = true
				}
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

		latest, err := pi.GetBlockByNumberWithLock(nil)
		if err != nil {
			zlog.Logger.Error().Err(err).Msgf("fail to get the latest height")
			bc.Broadcast(nil)
			return
		}
		blockHeight := int64(latest.NumberU64()) / ROUNDBLOCK
		signature, err := pi.TssSignBatch(allsignMSgs, previousPool.Pk, blockHeight)
		bc.Broadcast(signature)
		return
	}()

	waitForFinish.Wait()

	if !emptyERC20Tokens.IsSet() {
		// we add this account to "retry" to ensure it is the empty account in the next balance check
		pi.AddMoveFundItem(previousPool, height+movefundretrygap)
		return false
	} else {
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
}
