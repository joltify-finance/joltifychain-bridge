package oppybridge

func (oc *OppyChainInstance) QueryPendingTx(addr string) []*OutboundTx {
	items := []*OutboundTx{}
	oc.pendingTx.Range(func(t, value any) bool {
		item := value.(*OutboundTx)
		if addr == item.FromAddress {
			items = append(items, item)
		}
		return true
	})
	return items
}
