package oppybridge

import (
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
	"testing"
)

func TestUpdateFee(t *testing.T) {

	oc := OppyChainInstance{
		inBoundGas:  atomic.NewInt64(0),
		outBoundFee: atomic.NewInt64(0),
	}

	oc.UpdateGas(32)
	oc.UpdatePubChainFee(200)

	assert.Equal(t, int64(32), oc.inBoundGas.Load())
	assert.Equal(t, int64(200), oc.outBoundFee.Load())

}
