package bridge

import (
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

func NeedUpdate(qcPools []*vaulttypes.PoolInfo, curPools []*common.PoolInfo) bool {
	// in the cached address, the latest is at index 1 while the query pool has the latest at index 0
	if curPools[0] == nil || curPools[1] == nil {
		return true
	}
	var qPools []common.PoolInfo
	for i := 0; i < 2; i++ {
		pk := qcPools[i].GetCreatePool().GetPoolPubKey()
		addr, err := misc.PoolPubKeyToEthAddress(pk)
		if err != nil {
			return false
		}
		v1 := common.PoolInfo{
			Pk:      pk,
			Address: addr,
		}
		qPools = append(qPools, v1)
	}
	if qPools[0].Address == curPools[1].Address && qPools[1].Address == curPools[0].Address {
		return false
	}
	return true
}
