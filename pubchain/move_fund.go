package pubchain

import (
	zlog "github.com/rs/zerolog/log"
	"sync"

	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
)

//MoveFound moves the fund for the public chain
func (pi *Instance) MoveFound(wg *sync.WaitGroup, blockHeight int64, previousPool *bcommon.PoolInfo) bool {
	// we get the latest pool address and move funds to the latest pool
	currentPool := pi.GetPool()
	emptyAccountForAllTokens := true

	// movefund according to the history tokenlist
	existedTokenAddresses := pi.TokenList.GetAllExistedTokenAddresses()
	for _, tokenAddr := range existedTokenAddresses {
		emptyAccount, err := pi.doMoveFunds(wg, previousPool, currentPool[1].EthAddress, blockHeight, tokenAddr)
		// once there exists one token in the current pool, then we need to addMoveFundItem
		if err != nil {
			zlog.Log().Err(err).Msgf("fail to move the fund from %v to %v for token %v", previousPool.EthAddress.String(), currentPool[1].EthAddress.String(), tokenAddr)
			continue
		}
		// once there exists non-empty token in the pool account, we have to addMoveFundItem
		if !emptyAccount {
			emptyAccountForAllTokens = false
		}
	}

	if !emptyAccountForAllTokens {
		// we add this account to "retry" to ensure it is the empty account in the next balance check
		pi.AddMoveFundItem(previousPool, pi.CurrentHeight)
	}
	return !emptyAccountForAllTokens
}
