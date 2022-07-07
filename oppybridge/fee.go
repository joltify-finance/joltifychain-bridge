package oppybridge

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"gitlab.com/oppy-finance/oppy-bridge/config"
)

func (oc *OppyChainInstance) GetPubChainGasPrice() int64 {
	return oc.outBoundGasPrice.Load()
}

// UpdateGas update the gas needed from the last tx
func (oc *OppyChainInstance) UpdateGas(gasUsed int64) {
	oc.inBoundGas.Store(gasUsed)
}

func (oc *OppyChainInstance) UpdatePubChainGasPrice(gasPrice int64) {
	oc.outBoundGasPrice.Store(gasPrice)
}

// GetGasEstimation get the gas estimation
func (oc *OppyChainInstance) GetGasEstimation() int64 {
	previousGasUsed := oc.inBoundGas.Load()
	gasUsedDec := sdk.NewDecFromIntWithPrec(sdk.NewIntFromUint64(uint64(previousGasUsed)), 0)
	gasWanted := gasUsedDec.Mul(sdk.MustNewDecFromStr(config.GASFEERATIO)).RoundInt64()
	_ = gasWanted
	// Todo we need to get the gas dynamically in future, if different node get different fee, tss will fail
	return 50000000
}
