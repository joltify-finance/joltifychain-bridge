package joltifybridge

import (
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common/math"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/config"
)

func (jc *JoltifyChainInstance) AddMoveFundItem(pool *common.PoolInfo, height int64) {
	jc.moveFundReq.Store(height, pool)
}

// popMoveFundItemAfterBlock pop a move fund item after give block duration
func (jc *JoltifyChainInstance) popMoveFundItemAfterBlock(currentBlockHeight int64) (*common.PoolInfo, int64) {
	min := int64(math.MaxInt64)
	jc.moveFundReq.Range(func(key, value interface{}) bool {
		h := key.(int64)
		if h <= min {
			min = h
		}
		return true
	})

	if min < math.MaxInt64 && (currentBlockHeight-min > config.MINCHECKBLOCKGAP) {
		item, _ := jc.moveFundReq.LoadAndDelete(min)
		return item.(*common.PoolInfo), min
	}
	return nil, 0
}

func (jc *JoltifyChainInstance) ExportItems() []*common.OutBoundReq {
	var items []*common.OutBoundReq
	jc.RetryOutboundReq.Range(func(_, value interface{}) bool {
		items = append(items, value.(*common.OutBoundReq))
		return true
	})
	return items
}

func (jc *JoltifyChainInstance) AddItem(req *common.OutBoundReq) {
	jc.RetryOutboundReq.Store(req.Index(), req)
}

func (jc *JoltifyChainInstance) PopItem(n int) []*common.OutBoundReq {
	var allkeys []*big.Int
	jc.RetryOutboundReq.Range(func(key, value interface{}) bool {
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

func (jc *JoltifyChainInstance) Size() int {
	i := 0
	jc.RetryOutboundReq.Range(func(key, value interface{}) bool {
		i += 1
		return true
	})
	return i
}
