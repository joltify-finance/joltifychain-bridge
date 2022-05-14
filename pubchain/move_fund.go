package pubchain

import (
	"html"
	"sync"

	zlog "github.com/rs/zerolog/log"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
)

//MoveFound moves the fund for the public chain
func (pi *Instance) MoveFound(wg *sync.WaitGroup, blockHeight int64, previousPool *bcommon.PoolInfo) bool {

	// we get the latest pool address and move funds to the latest pool
	currentPool := pi.GetPool()
	successful := false
	pi.tokenList.Range(func(k, v interface{}) bool {
		tokenInfo := v.(bcommon.TokenInfo)
		emptyAccount, err := pi.doMoveFunds(wg, previousPool, currentPool[1].EthAddress, blockHeight, tokenInfo)
		if err != nil {
			zlog.Log().Err(err).Msgf("fail to move the fund from %v to %v", previousPool.EthAddress.String(), currentPool[1].EthAddress.String())
			pi.AddMoveFundItem(previousPool, pi.CurrentHeight)
			successful = true
			return true
		}
		if emptyAccount {
			tick := html.UnescapeString("&#" + "9989" + ";")
			zlog.Logger.Info().Msgf("%v account %v is clear no need to move", tick, previousPool.EthAddress.String())
			successful = false
			return true
		}

		// we add this account to "retry" to ensure it is the empty account in the next balance check
		pi.AddMoveFundItem(previousPool, pi.CurrentHeight)
		successful = true
		return true
	})

	return successful

}
