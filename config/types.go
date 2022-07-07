package config

import "time"

const (
	OutBoundDenomFee = "abnb"
	NativeToken      = "abnb"

	OUTBoundFeeOut           = "0.000000000001"
	TxTimeout                = 300
	GASFEERATIO              = "1.5"
	DUSTBNB                  = "0.0001"
	MINCHECKBLOCKGAP         = 6
	DefaultPUBChainGasWanted = "25000"
)

var ChainID = "oppyChain-1"

const (
	InBound = iota
	OutBound
	QueryTimeOut = time.Second * 6
)

// direction is the direction of the oppy_bridge
type Direction int
