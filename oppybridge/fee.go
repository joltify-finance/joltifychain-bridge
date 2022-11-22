package oppybridge

func (oc *OppyChainInstance) GetPubChainFee(chainName string) int64 {
	return oc.outBoundFeeMap[chainName].Load()
}

// UpdateGas update the gas needed from the last tx
func (oc *OppyChainInstance) UpdateGas(gasUsed int64) {
	oc.inBoundGas.Store(gasUsed)
}

func (oc *OppyChainInstance) UpdatePubChainFee(gasPrice int64, chainName string) {
	oc.outBoundFeeMap[chainName].Store(gasPrice)
}
