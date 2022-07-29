package oppybridge

import (
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common/math"
	"gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/config"
)

func (oc *OppyChainInstance) AddMoveFundItem(pool *common.PoolInfo, height int64) {
	oc.moveFundReq.Store(height, pool)
}

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

func (oc *OppyChainInstance) ExportMoveFundItems() []*common.PoolInfo {
	var data []*common.PoolInfo
	oc.moveFundReq.Range(func(key, value any) bool {
		exported := value.(*common.PoolInfo)
		exported.Height = key.(int64)
		data = append(data, exported)
		return true
	})
	return data
}

// popMoveFundItemAfterBlock pop a move fund item after give block duration
func (oc *OppyChainInstance) popMoveFundItemAfterBlock(currentBlockHeight int64) (*common.PoolInfo, int64) {
	min := int64(math.MaxInt64)
	oc.moveFundReq.Range(func(key, value interface{}) bool {
		h := key.(int64)
		if h <= min {
			min = h
		}
		return true
	})

	if min < math.MaxInt64 && (currentBlockHeight-min > config.MINCHECKBLOCKGAP) {
		item, _ := oc.moveFundReq.LoadAndDelete(min)
		return item.(*common.PoolInfo), min
	}
	return nil, 0
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

func (oc *OppyChainInstance) PopItem(n int) []*common.OutBoundReq {
	var allkeys []string
	oc.RetryOutboundReq.Range(func(key, value interface{}) bool {
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

func (oc *OppyChainInstance) Size() int {
	i := 0
	oc.RetryOutboundReq.Range(func(key, value interface{}) bool {
		i += 1
		return true
	})
	return i
}
