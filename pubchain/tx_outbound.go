package pubchain

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// SendToken sends the token to the public chain
func (pi *PubChainInstance) SendToken(sender, receiver common.Address, amount *big.Int, blockHeight int64) error {
	tokenInstance := pi.tokenSb.tokenInstance
	ctx, cancel := context.WithTimeout(context.Background(), chainQueryTimeout)
	defer cancel()
	chainID, err := pi.EthClient.NetworkID(ctx)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to get the chain ID")
		return err
	}

	txo, err := pi.composeTx(sender, chainID, blockHeight)
	if err != nil {
		return err
	}
	txo.GasLimit = GasLimit

	ret, err := tokenInstance.Transfer(txo, receiver, amount)
	if err != nil {
		pi.logger.Error().Err(err).Msg("fail to call the multisend")
		return err
	}
	pi.logger.Info().Msgf("we have done the outbound tx %v", ret.Hash())
	return nil
}

// ProcessOutBound send the money to public chain
func (pi *PubChainInstance) ProcessOutBound(toAddr, fromAddr common.Address, amount *big.Int, blockHeight int64) error {
	err := pi.SendToken(fromAddr, toAddr, amount, blockHeight)
	if err != nil {
		pi.logger.Error().Err(err).Msgf("fail to send the token with err %v", err)
		return err
	}
	return nil
}
