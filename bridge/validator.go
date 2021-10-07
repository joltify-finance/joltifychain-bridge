package bridge

func (ic *InvChainBridge) InitValidators() error {
	_, err := QueryHistoricalValidator(ic.grpcClient)
	if err != nil {
		ic.logger.Error().Err(err).Msg("fail to initialize the validator pool")
		return err
	}
	return nil
}
