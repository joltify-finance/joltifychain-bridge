package pubchain

import (
	"math/big"
	"sort"

	"gitlab.com/oppy-finance/oppy-bridge/common"
)

func (pi *Instance) AddInBoundItem(req *common.InBoundReq) {
	pi.RetryInboundReq.Store(req.Index(), req)
}

func (pi *Instance) AddOnHoldQueue(item *common.InBoundReq) {
	pi.onHoldRetryQueueLock.Lock()
	defer pi.onHoldRetryQueueLock.Unlock()
	pi.onHoldRetryQueue = append(pi.onHoldRetryQueue, item)
}

func (pi *Instance) DumpQueue() []*common.InBoundReq {
	pi.onHoldRetryQueueLock.Lock()
	defer pi.onHoldRetryQueueLock.Unlock()
	if len(pi.onHoldRetryQueue) == 0 {
		return []*common.InBoundReq{}
	}
	ret := pi.onHoldRetryQueue
	pi.onHoldRetryQueue = []*common.InBoundReq{}
	return ret
}

func (pi *Instance) ExportItems() []*common.InBoundReq {
	var items []*common.InBoundReq
	pi.RetryInboundReq.Range(func(_, value interface{}) bool {
		items = append(items, value.(*common.InBoundReq))
		return true
	})
	return items
}

func (pi *Instance) AddOutBoundItem(req *common.OutBoundReq) {
	pi.RetryOutboundReq.Store(req.Index(), req)
}

func (pi *Instance) IsEmpty() bool {
	empty := true
	pi.RetryInboundReq.Range(func(key, value any) bool {
		empty = false
		return false
	})
	return empty
}

func (pi *Instance) PopItem(n int) []*common.InBoundReq {
	var allkeys []string
	pi.RetryInboundReq.Range(func(key, value interface{}) bool {
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

	inboundReqs := make([]*common.InBoundReq, returnNum)

	pi.logger.Warn().Msgf("the pop out items seq array is %v----all in queue (%v)", allkeys[:returnNum], len(allkeys))
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
