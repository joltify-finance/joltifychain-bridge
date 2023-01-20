package config

import "time"

const (
	NativeSign = "native"

	GASFEERATIO      = "2"
	DEFAULTNATIVEGAS = 21000
	MINCHECKBLOCKGAP = 30
)

var ChainID = "joltifymock-1"

const (
	QueryTimeOut = time.Second * 6
)

// Direction is the direction of the oppy_bridge
type Direction int
