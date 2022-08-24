package config

import "time"

const (
	OutBoundDenomFee = "abnb"
	NativeSign       = "native"

	TxTimeout                   = 300
	GASFEERATIO                 = "1.5"
	PubChainGASFEERATIO         = 3
	MoveFundPubChainGASFEERATIO = 1.2
	MINCHECKBLOCKGAP            = 6
)

var ChainID = "oppyChain-1"

const (
	QueryTimeOut = time.Second * 6
)

// Direction is the direction of the oppy_bridge
type Direction int
