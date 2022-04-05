package monitor

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMetricInbound(t *testing.T) {
	metrics := NewMetric()
	metrics.UpdateInboundTxNum(1)
	assert.Equal(t, metrics.inboundTxNum, 1)

	metrics.UpdateInboundTxNum(10)
	assert.Equal(t, metrics.inboundTxNum, 10)

	metrics.UpdateInboundTxNum(2)
	assert.Equal(t, metrics.inboundTxNum, 2)
}

func TestMetricOutbound(t *testing.T) {
	metrics := NewMetric()
	metrics.UpdateOutboundTxNum(1)
	assert.Equal(t, metrics.outboundTxNum, 1)

	metrics.UpdateOutboundTxNum(10)
	assert.Equal(t, metrics.outboundTxNum, 10)

	metrics.UpdateOutboundTxNum(2)
	assert.Equal(t, metrics.outboundTxNum, 2)
}
