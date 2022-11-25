package cosbridge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
)

func TestUpdateFee(t *testing.T) {
	oc := OppyChainInstance{
		inBoundGas:     atomic.NewInt64(0),
		outBoundFeeMap: make(map[string]*atomic.Int64),
	}

	oc.outBoundFeeMap["BSC"] = atomic.NewInt64(0)
	oc.UpdateGas(32)
	oc.UpdatePubChainFee(200, "BSC")

	assert.Equal(t, int64(32), oc.inBoundGas.Load())
}
