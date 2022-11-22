package oppybridge

import (
	"math/big"
	"sort"

	"gitlab.com/oppy-finance/oppy-bridge/common"
)

func (oc *OppyChainInstance) AddOnHoldQueue(item *common.OutBoundReq) {
	oc.onHoldRetryQueueLock.Lock()
	defer oc.onHoldRetryQueueLock.Unlock()
	oc.onHoldRetryQueue = append(oc.onHoldRetryQueue, item)
}

func (oc *OppyChainInstance) DumpQueue() []*common.OutBoundReq {
	oc.onHoldRetryQueueLock.Lock()
	defer oc.onHoldRetryQueueLock.Unlock()
	if len(oc.onHoldRetryQueue) == 0 {
		return []*common.OutBoundReq{}
	}
	ret := oc.onHoldRetryQueue
	oc.onHoldRetryQueue = []*common.OutBoundReq{}
	return ret
}

func (oc *OppyChainInstance) ExportItems() []*common.OutBoundReq {
	var items []*common.OutBoundReq
	oc.RetryOutboundReq.Range(func(_, value interface{}) bool {
		items = append(items, value.(*common.OutBoundReq))
		return true
	})
	return items
}

func (oc *OppyChainInstance) Import(items []*OutboundTx) {
	for _, el := range items {
		oc.pendingTx.Store(el.TxID, el)
	}
}

func (oc *OppyChainInstance) Export() []*OutboundTx {
	var exported []*OutboundTx
	oc.pendingTx.Range(func(key, value any) bool {
		exported = append(exported, value.(*OutboundTx))
		return true
	})
	return exported
}

func (oc *OppyChainInstance) AddItem(req *common.OutBoundReq) {
	oc.RetryOutboundReq.Store(req.Index(), req)
}

func (oc *OppyChainInstance) PopItem(n int, chainType string) []*common.OutBoundReq {
	var allkeys []string
	oc.RetryOutboundReq.Range(func(key, value interface{}) bool {
		req := value.(*common.OutBoundReq)
		if req.ChainType != chainType {
			return true
		}
		allkeys = append(allkeys, key.(string))
		return true
	})

	sort.Slice(allkeys, func(i, j int) bool {
		a, _ := new(big.Int).SetString(allkeys[i], 10)
		b, _ := new(big.Int).SetString(allkeys[j], 10)
		return a.Cmp(b) == -1
	})
	indexNum := len(allkeys)
	if indexNum == 0 {
		return nil
	}

	returnNum := n
	if indexNum < n {
		returnNum = indexNum
	}

	inboundReqs := make([]*common.OutBoundReq, returnNum)

	for i := 0; i < returnNum; i++ {
		el, loaded := oc.RetryOutboundReq.LoadAndDelete(allkeys[i])
		if !loaded {
			panic("should never fail")
		}
		inboundReqs[i] = el.(*common.OutBoundReq)
	}

	return inboundReqs
}

func (oc *OppyChainInstance) IsEmpty() bool {
	empty := true
	oc.RetryOutboundReq.Range(func(key, value interface{}) bool {
		empty = false
		return false
	})
	return empty
}

func (oc *OppyChainInstance) Size() int {
	i := 0
	oc.RetryOutboundReq.Range(func(key, value interface{}) bool {
		i++
		return true
	})
	return i
}
