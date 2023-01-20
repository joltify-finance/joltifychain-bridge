package cosbridge

import (
	"math/big"
	"sort"

	"gitlab.com/joltify/joltifychain-bridge/common"
)

func (jc *JoltChainInstance) AddOnHoldQueue(item *common.OutBoundReq) {
	jc.onHoldRetryQueueLock.Lock()
	defer jc.onHoldRetryQueueLock.Unlock()
	jc.onHoldRetryQueue = append(jc.onHoldRetryQueue, item)
}

func (jc *JoltChainInstance) RetrieveItemsWithType(chainType string) []*common.OutBoundReq {
	jc.onHoldRetryQueueLock.Lock()
	defer jc.onHoldRetryQueueLock.Unlock()
	if len(jc.onHoldRetryQueue) == 0 {
		return []*common.OutBoundReq{}
	}
	var ret, newQueue []*common.OutBoundReq
	for _, el := range jc.onHoldRetryQueue {
		if el.ChainType == chainType {
			ret = append(ret, el)
		} else {
			newQueue = append(newQueue, el)
		}
	}
	jc.onHoldRetryQueue = newQueue
	return ret
}

func (jc *JoltChainInstance) DumpQueue() []*common.OutBoundReq {
	jc.onHoldRetryQueueLock.Lock()
	defer jc.onHoldRetryQueueLock.Unlock()
	if len(jc.onHoldRetryQueue) == 0 {
		return []*common.OutBoundReq{}
	}
	ret := jc.onHoldRetryQueue
	jc.onHoldRetryQueue = []*common.OutBoundReq{}
	return ret
}

func (jc *JoltChainInstance) ExportItems() []*common.OutBoundReq {
	var items []*common.OutBoundReq
	jc.RetryOutboundReq.Range(func(_, value interface{}) bool {
		items = append(items, value.(*common.OutBoundReq))
		return true
	})
	return items
}

func (jc *JoltChainInstance) AddItem(req *common.OutBoundReq) {
	jc.RetryOutboundReq.Store(req.Index(), req)
}

func (jc *JoltChainInstance) PopItem(n int, chainType string) []*common.OutBoundReq {
	var allkeys []string
	jc.RetryOutboundReq.Range(func(key, value interface{}) bool {
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
		el, loaded := jc.RetryOutboundReq.LoadAndDelete(allkeys[i])
		if !loaded {
			panic("should never fail")
		}
		inboundReqs[i] = el.(*common.OutBoundReq)
	}

	return inboundReqs
}

func (jc *JoltChainInstance) IsEmpty() bool {
	empty := true
	jc.RetryOutboundReq.Range(func(key, value interface{}) bool {
		empty = false
		return false
	})
	return empty
}

func (jc *JoltChainInstance) Size() int {
	i := 0
	jc.RetryOutboundReq.Range(func(key, value interface{}) bool {
		i++
		return true
	})
	return i
}
