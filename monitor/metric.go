package monitor

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Metric struct {
	inboundTxNum  prometheus.Gauge
	outboundTxNum prometheus.Gauge
	status        prometheus.Gauge
	logger        zerolog.Logger
}

func (m *Metric) UpdateInboundTxNum(num float64) {
	m.inboundTxNum.Set(num)
}

func (m *Metric) UpdateOutboundTxNum(num float64) {
	m.outboundTxNum.Set(num)
}

func (m *Metric) UpdateStatus() {
	m.status.SetToCurrentTime()
}

func (m *Metric) Enable() {
	prometheus.MustRegister(m.inboundTxNum)
	prometheus.MustRegister(m.outboundTxNum)
	prometheus.MustRegister(m.status)
}

func NewMetric() *Metric {
	metrics := Metric{
		inboundTxNum: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "Oppy",
				Subsystem: "bridge",
				Name:      "inbound_tx",
				Help:      "the number of tx in inbound queue",
			},
		),

		outboundTxNum: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "Oppy",
				Subsystem: "bridge",
				Name:      "outbound_tx",
				Help:      "the number of tx in outbound queue",
			},
		),
		status: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "Oppy",
				Subsystem: "bridge",
				Name:      "bridge_status",
				Help:      "the latest time that the bridge is alive",
			},
		),

		logger: log.With().Str("module", "oppyMonitor").Logger(),
	}
	return &metrics
}
