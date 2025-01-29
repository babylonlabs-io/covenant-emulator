package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type CovenantSignerMetrics struct {
	Registry                   *prometheus.Registry
	ReceivedSigningRequests    prometheus.Counter
	SuccessfulSigningRequests  prometheus.Counter
	FailedSigningRequests      prometheus.Counter
	SignerUnlockStatus         prometheus.Gauge
	SignerFailedUnlockRequests prometheus.Counter
}

func NewCovenantSignerMetrics() *CovenantSignerMetrics {
	registry := prometheus.NewRegistry()
	registerer := promauto.With(registry)

	uwMetrics := &CovenantSignerMetrics{
		Registry: registry,
		ReceivedSigningRequests: registerer.NewCounter(prometheus.CounterOpts{
			Name: "signer_received_signing_requests",
			Help: "The total number of signing requests received by the signer",
		}),
		SuccessfulSigningRequests: registerer.NewCounter(prometheus.CounterOpts{
			Name: "signer_succeeded_signing_requests",
			Help: "The total number times the signer successfully responded with a signature",
		}),
		FailedSigningRequests: registerer.NewCounter(prometheus.CounterOpts{
			Name: "signer_failed_signing_requests",
			Help: "The total number of times signer responded with an internal error",
		}),
		SignerUnlockStatus: registerer.NewGauge(prometheus.GaugeOpts{
			Name: "signer_unlock_status",
			Help: "The status indicating if the signer is unlocked or locked. 1 for unlocked, 0 for locked",
		}),
		SignerFailedUnlockRequests: registerer.NewCounter(prometheus.CounterOpts{
			Name: "signer_failed_unlock_requests",
			Help: "The total number of times the signer failed to unlock",
		}),
	}

	return uwMetrics
}

func (m *CovenantSignerMetrics) IncReceivedSigningRequests() {
	m.ReceivedSigningRequests.Inc()
}

func (m *CovenantSignerMetrics) IncSuccessfulSigningRequests() {
	m.SuccessfulSigningRequests.Inc()
}

func (m *CovenantSignerMetrics) IncFailedSigningRequests() {
	m.FailedSigningRequests.Inc()
}

func (m *CovenantSignerMetrics) SetSignerUnlocked() {
	m.SignerUnlockStatus.Set(1)
}

func (m *CovenantSignerMetrics) SetSignerLocked() {
	m.SignerUnlockStatus.Set(0)
}

func (m *CovenantSignerMetrics) IncFailedUnlockRequests() {
	m.SignerFailedUnlockRequests.Inc()
}
