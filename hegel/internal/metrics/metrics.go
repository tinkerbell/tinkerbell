package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	Started = iota
	Initializing
	Ready
)

var (
	CacherConnected    prometheus.Gauge
	CacherHealthcheck  *prometheus.CounterVec
	InitDuration       prometheus.Observer
	Errors             *prometheus.CounterVec
	MetadataRequests   prometheus.Counter
	State              prometheus.Gauge
	Subscriptions      *prometheus.GaugeVec
	TotalSubscriptions prometheus.Counter
)

func init() {
	CacherConnected = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "hegel_cacher_connected",
		Help: "Hegel health check status for cacher, 0:not connected, 1:connected",
	})

	CacherHealthcheck = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hegel_cacher_healthchecks_total",
		Help: "Total count of healthchecks to cacher",
	}, []string{"success"})

	labelValues := []prometheus.Labels{
		{"success": "true"},
		{"success": "false"},
	}
	initCounterLabels(CacherHealthcheck, labelValues)

	InitDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hegel_subscription_initialization_duration_seconds",
		Help:    "Duration taken to get a response for a newly discovered request.",
		Buckets: []float64{0.5, 1, 5, 10, 30, 60},
	}, []string{}).With(prometheus.Labels{})

	Errors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hegel_errors",
		Help: "Number of errors tracked by hegel",
	}, []string{"op", "state"})

	labelValues = []prometheus.Labels{
		{"op": "cacher", "state": "healthcheck"},
		{"op": "metadata", "state": "lookup"},
		{"op": "subscribe", "state": "active"},
		{"op": "subscribe", "state": "initializing"},
	}
	initCounterLabels(Errors, labelValues)

	MetadataRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hegel_metadata_requests_total",
		Help: "Number of requests to the metadata http endpoint",
	})

	State = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "hegel_state",
		Help: "Current state of hegel, 0:started, 1:initializing, 2:ready",
	})

	Subscriptions = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hegel_subscriptions",
		Help: "Number of hegel subscribers",
	}, []string{"state"})

	labelValues = []prometheus.Labels{
		{"state": "active"},
		{"state": "initializing"},
	}
	initGaugeLabels(Subscriptions, labelValues)

	TotalSubscriptions = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hegel_subscriptions_total",
		Help: "Total number of connections hegel has handled",
	})
}

func initCounterLabels(m *prometheus.CounterVec, l []prometheus.Labels) {
	for _, labels := range l {
		m.With(labels)
	}
}

func initGaugeLabels(m *prometheus.GaugeVec, l []prometheus.Labels) {
	for _, labels := range l {
		m.With(labels)
	}
}
