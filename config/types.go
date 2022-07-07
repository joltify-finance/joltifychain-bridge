package config

import "time"

const (
	OutBoundDenomFee = "abnb"
	NativeSign       = "native"

	TxTimeout                = 300
	GASFEERATIO              = "1.5"
	DUSTBNB                  = "0.0001"
	MINCHECKBLOCKGAP         = 6
	DefaultPUBChainGasWanted = "100000"
)

var ChainID = "oppyChain-1"

const (
	InBound = iota
	OutBound
	QueryTimeOut = time.Second * 6
)

// direction is the direction of the oppy_bridge
type Direction int
