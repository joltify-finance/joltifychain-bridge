package config

import "time"

const (
	InBoundDenomFee  = "BNB"
	OutBoundDenomFee = "JOLT"
	InBoundFeeMin    = "0.000000000000000001"
	OUTBoundFeeOut   = "0.000000000000000001"
	InBoundDenom     = "JUSD"
	OutBoundDenom    = "JUSD"
	TxTimeout        = 300
)

const (
	InBound = iota
	OutBound
	QueryTimeOut = time.Second * 3
)

// direction is the direction of the joltify_bridge
type Direction int
