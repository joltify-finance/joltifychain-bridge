package oppybridge

func (oc *OppyChainInstance) GetPubChainFee() int64 {
	return oc.outBoundFee.Load()
}

// UpdateGas update the gas needed from the last tx
func (oc *OppyChainInstance) UpdateGas(gasUsed int64) {
	oc.inBoundGas.Store(gasUsed)
}

func (oc *OppyChainInstance) UpdatePubChainFee(gasPrice int64) {
	oc.outBoundFee.Store(gasPrice)
}
