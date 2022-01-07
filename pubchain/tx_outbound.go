package pubchain

import (
	"context"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ethereum/go-ethereum/common"
)

// SendToken sends the token to the public chain
func (pi *PubChainInstance) SendToken(sender, receiver common.Address, amount *big.Int, blockHeight int64) (string, error) {
	tokenInstance := pi.tokenSb.tokenInstance
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	chainID, err := pi.EthClient.NetworkID(ctx)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to get the chain ID")
		return "", err
	}
	txo, err := pi.composeTx(sender, chainID, blockHeight)
	if err != nil {
		return "", err
	}

	ret, err := tokenInstance.Transfer(txo, receiver, amount)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to send the token to the address %v", receiver)
		return "", err
	}
	pi.logger.Info().Msgf("we have done the outbound tx %v", ret.Hash().Hex())
	return ret.Hash().Hex(), nil
}

// ProcessOutBound send the money to public chain
func (pi *PubChainInstance) ProcessOutBound(toAddr, fromAddr common.Address, amount *big.Int, blockHeight int64) (string, error) {
	pi.logger.Info().Msgf(">>>>from addr %v to addr %v with amount %v\n", fromAddr, toAddr, sdk.NewDecFromBigIntWithPrec(amount, 18))
	txHash, err := pi.SendToken(fromAddr, toAddr, amount, blockHeight)
	if err != nil {
		if err.Error() == "already known" {
			pi.logger.Warn().Msgf("the tx has been submitted by others")
			return txHash, nil
		}
		pi.logger.Error().Err(err).Msgf("fail to send the token with err %v", err)
		return "", err
	}
	return txHash, nil
}
