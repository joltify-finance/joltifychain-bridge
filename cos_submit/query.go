package cossubmit

import (
	"context"

	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
)

// GetLastBlockHeightWithLock gets the current block height
func (cs *CosHandler) GetLastBlockHeightWithLock() (int64, error) {
	cs.GrpcLock.Lock()
	b, err := bcommon.GetLastBlockHeight(cs.GrpcClient)
	cs.GrpcLock.Unlock()
	if err != nil {
		err2 := cs.RetryJoltifyChain(false)
		if err2 != nil {
			cs.logger.Error().Err(err).Msgf("we fail to reset the oppychain")
		}
	}
	return b, err
}

func (cs *CosHandler) GetValidators(height string) ([]*vaulttypes.Validator, error) {
	cs.GrpcLock.Lock()
	vaultQuery := vaulttypes.NewQueryClient(cs.GrpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	q := vaulttypes.QueryGetValidatorsRequest{Height: height}
	vaultResp, err := vaultQuery.GetValidators(ctx, &q)
	cs.GrpcLock.Unlock()
	if err != nil {
		cs.logger.Error().Err(err).Msg("fail to query the validators")
		return nil, err
	}
	return vaultResp.Validators.AllValidators, nil
}
