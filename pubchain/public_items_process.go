package pubchain

import (
	"math/big"
	"sort"

	"gitlab.com/oppy-finance/oppy-bridge/common"
)

func (pi *Instance) AddItem(req *common.InBoundReq) {
	pi.RetryInboundReq.Store(req.Index(), req)
}

func (pi *Instance) ExportItems() []*common.InBoundReq {
	var items []*common.InBoundReq
	pi.RetryInboundReq.Range(func(_, value interface{}) bool {
		items = append(items, value.(*common.InBoundReq))
		return true
	})
	return items
}

func (pi *Instance) AddPendingTx(pendingTx *InboundTx) {
	pi.pendingInbounds.Store(pendingTx.TxID, pendingTx)
}

func (pi *Instance) AddPendingTxBnb(pendingTxBnb *InboundTxBnb) {
	pi.pendingInboundsBnB.Store(pendingTxBnb.TxID, pendingTxBnb)
}

func (pi *Instance) ExportPendingItems() []*InboundTx {
	var items []*InboundTx
	pi.pendingInbounds.Range(func(_, value interface{}) bool {
		items = append(items, value.(*InboundTx))
		return true
	})
	return items
}

func (pi *Instance) ExportPendingBnbItems() []*InboundTxBnb {
	var items []*InboundTxBnb
	pi.pendingInboundsBnB.Range(func(key, value interface{}) bool {
		items = append(items, value.(*InboundTxBnb))
		return true
	})
	return items
}

func (pi *Instance) PopItem(n int) []*common.InBoundReq {
	var allkeys []*big.Int
	pi.RetryInboundReq.Range(func(key, value interface{}) bool {
		allkeys = append(allkeys, key.(*big.Int))
		return true
	})

	sort.Slice(allkeys, func(i, j int) bool {
		return allkeys[i].Cmp(allkeys[j]) == -1
	})
	indexNum := len(allkeys)
	if indexNum == 0 {
		return nil
	}

	returnNum := n
	if indexNum < n {
		returnNum = indexNum
	}

	inboundReqs := make([]*common.InBoundReq, returnNum)

	for i := 0; i < returnNum; i++ {
		el, loaded := pi.RetryInboundReq.LoadAndDelete(allkeys[i])
		if !loaded {
			panic("should never fail")
		}
		inboundReqs[i] = el.(*common.InBoundReq)
	}

	return inboundReqs
}

func (pi *Instance) Size() int {
	i := 0
	pi.RetryInboundReq.Range(func(key, value interface{}) bool {
		i++
		return true
	})
	return i
}
