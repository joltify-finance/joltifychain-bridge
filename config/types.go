package config

import "time"

const (
	NativeSign = "native"

	TxTimeout                   = 300
	GASFEERATIO                 = "5"
	PubChainGASFEERATIO         = 3
	MoveFundPubChainGASFEERATIO = 1
	MINCHECKBLOCKGAP            = 30
)

var ChainID = "joltifymock-1"

const (
	QueryTimeOut = time.Second * 6
)

// Direction is the direction of the oppy_bridge
type Direction int
