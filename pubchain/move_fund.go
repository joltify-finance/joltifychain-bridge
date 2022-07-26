package pubchain

import (
	"sort"
	"sync"

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
	emptyERC20Tokens := true

	// movefund according to the history tokenlist
	existedTokenAddresses := pi.TokenList.GetAllExistedTokenAddresses()
	sort.Strings(existedTokenAddresses)
	for _, tokenAddr := range existedTokenAddresses {
		if tokenAddr == "native" {
			continue
		}
		// if we restart the bridge, pubchain go routine may run before oppy go routine which acquire the pool info
		if currentPool[1] == nil {
			zlog.Warn().Msgf("the current pool has not been set, move fund can not start")
			return false
		}

		tokenIsEmpty, err := pi.doMoveTokenFunds(wg, previousPool, currentPool[1].EthAddress, tokenAddr, ethClient)
		// once there exists one token in the current pool, then we need to addMoveFundItem
		if err != nil {
			zlog.Log().Err(err).Msgf("fail to move the fund from %v to %v for token %v", previousPool.EthAddress.String(), currentPool[1].EthAddress.String(), tokenAddr)
			emptyERC20Tokens = false
			continue
		}

		// once there exists non-empty token in the pool account, we have to addMoveFundItem
		if !tokenIsEmpty {
			emptyERC20Tokens = false
		}
	}

	if !emptyERC20Tokens {
		// we add this account to "retry" to ensure it is the empty account in the next balance check
		pi.AddMoveFundItem(previousPool, height+movefundretrygap)
		return false
	} else {

		bnbIsMoved, isEmpty, err := pi.doMoveBNBFunds(wg, previousPool, currentPool[1].EthAddress)
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
