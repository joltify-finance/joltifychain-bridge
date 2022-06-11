package bridge

import (
	"gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
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
			Pk:          pk,
			OppyAddress: addr,
			EthAddress:  ethAddr,
		}
		qPools = append(qPools, v1)
	}
	if qPools[0].OppyAddress.Equals(curPools[1].OppyAddress) && qPools[1].OppyAddress.Equals(curPools[0].OppyAddress) {
		return false
	}
	return true
}
