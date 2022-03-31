package monitor

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Metric struct {
	inboundTxNum  prometheus.Gauge
	outboundTxNum prometheus.Gauge
	logger        zerolog.Logger
}

func (m *Metric) UpdateInboundTxNum(num float64) {
	m.inboundTxNum.Set(num)
}

func (m *Metric) UpdateOutboundTxNum(num float64) {
	m.outboundTxNum.Set(num)
}

func (m *Metric) Enable() {
	prometheus.MustRegister(m.inboundTxNum)
	prometheus.MustRegister(m.outboundTxNum)
}

func NewMetric() *Metric {
	metrics := Metric{

		inboundTxNum: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "Joltify",
				Subsystem: "bridge",
				Name:      "inbound_tx",
				Help:      "the number of tx in inbound queue",
			},
		),

		outboundTxNum: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "Joltify",
				Subsystem: "bridge",
				Name:      "outbound_tx",
				Help:      "the number of tx in outbound queue",
			},
		),

		logger: log.With().Str("module", "joltifyMonitor").Logger(),
	}
	return &metrics
}
