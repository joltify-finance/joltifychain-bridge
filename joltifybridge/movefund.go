package joltifybridge

import (
	"github.com/cosmos/cosmos-sdk/types"
	zlog "github.com/rs/zerolog/log"
	"html"
)

//MoveFound move all the funds for joltify chain
func (jc *JoltifyChainInstance) MoveFound(currentBlockHeight int64, toAddress types.AccAddress) bool {
	moveFound := false
	// we move fund if some pool retired
	previousPool, _ := jc.popMoveFundItemAfterBlock(currentBlockHeight)
	if previousPool == nil {
		return moveFound
	}
	// we get the latest pool address and move funds to the latest pool
	isSigner, err := jc.CheckWhetherSigner(previousPool.PoolInfo)
	if err != nil {
		jc.logger.Warn().Msg("fail in check whether we are signer in moving fund")
		return moveFound
	}
	if !isSigner {
		jc.logger.Info().Msgf("we are not the signer, no need to move funds")
		return moveFound
	}
	moveFound = true
	emptyAcc, err := jc.MoveFunds(previousPool, toAddress, currentBlockHeight)
	if emptyAcc {
		tick := html.UnescapeString("&#" + "127974" + ";")
		zlog.Logger.Info().Msgf("%v successfully moved funds from %v to %v", tick, previousPool.JoltifyAddress.String(), toAddress.String())
		return moveFound
	}
	if err != nil {
		zlog.Log().Err(err).Msgf("fail to move the fund from %v to %v", previousPool.JoltifyAddress.String(), toAddress.String())
	}
	jc.logger.Error().Msgf("fail to move fund for this round, will retry")
	jc.AddMoveFundItem(previousPool, currentBlockHeight)
	return moveFound
}
