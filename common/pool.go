package common

import (
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

// PoolInfo stores the pool and pk of the joltify pool
type PoolInfo struct {
	Pk             string
	JoltifyAddress types.AccAddress
	EthAddress     common.Address
	PoolInfo       *vaulttypes.PoolInfo
}
