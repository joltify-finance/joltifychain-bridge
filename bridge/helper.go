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
		ethAddr, err := misc.PoolPubKeyToEthAddress(pk)
		if err != nil {
			return false
		}
		addr, err := misc.PoolPubKeyToJoltAddress(pk)
		if err != nil {
			return false
		}
		v1 := common.PoolInfo{
			Pk:             pk,
			JoltifyAddress: addr,
			EthAddress:     ethAddr,
		}
		qPools = append(qPools, v1)
	}
	if qPools[0].JoltifyAddress.Equals(curPools[1].JoltifyAddress) && qPools[1].JoltifyAddress.Equals(curPools[0].JoltifyAddress) {
		return false
	}
	return true
}
