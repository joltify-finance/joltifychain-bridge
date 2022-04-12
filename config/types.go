package config

import "time"

const (
	InBoundDenomFee = "BNB"

	OutBoundDenomFee = "JOLT"

	InBoundFeeMin    = "0.00000000000000001"
	OUTBoundFeeOut   = "0.00000000000000001"
	InBoundDenom     = "JUSD"
	OutBoundDenom    = "JUSD"
	TxTimeout        = 300
	GASFEERATIO      = "1.5"
	DUSTBNB          = "0.0001"
	MINCHECKBLOCKGAP = 6
)

var ChainID = "joltifyChain-1"

const (
	InBound = iota
	OutBound
	QueryTimeOut = time.Second * 6
)

// direction is the direction of the joltify_bridge
type Direction int
