package bridge

import (
	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

func NeedUpdate(qcPools []*vaulttypes.PoolInfo, curPools []*common.PoolInfo) bool {
	// in the cached address, the latest is at index 1 while the query latest pool has the latest at index 0
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
		addr, err := misc.PoolPubKeyToJoltifyAddress(pk)
		if err != nil {
			return false
		}
		v1 := common.PoolInfo{
			Pk:         pk,
			CosAddress: addr,
			EthAddress: ethAddr,
		}
		qPools = append(qPools, v1)
	}
	if qPools[0].CosAddress.Equals(curPools[1].CosAddress) && qPools[1].CosAddress.Equals(curPools[0].CosAddress) {
		return false
	}
	return true
}
