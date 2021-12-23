package common

import "github.com/ethereum/go-ethereum/common"

//PoolInfo stores the pool and pk of the joltify pool
type PoolInfo struct {
	Pk      string
	Address common.Address
}
