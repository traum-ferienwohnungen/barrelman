package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// Prometheus metrics
	NodeCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "barrelman_current_nodes_count",
		Help: "Number of nodes in watched cluster.",
	})
	EndpointUpdates = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "barrelman_endpoint_update_total",
		Help: "Count of service endpoints updates",
	})
	EndpointUpdateErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "barrelman_endpoint_update_error_total",
		Help: "Count of errors during endpoints updates",
	})
	ServiceCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "barrelman_current_services_count",
		Help: "Number of barrelman services in watched cluster.",
	})
	ServiceUpdates = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "barrelman_service_update_total",
			Help: "Count of service updates",
		},
		[]string{"action"},
	)
	ServiceUpdateErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "barrelman_service_update_error_total",
			Help: "Count of errors during service updates",
		},
		[]string{"action"},
	)
)

func init() {
	// Register prometheus metrics
	prometheus.MustRegister(NodeCount)
	prometheus.MustRegister(EndpointUpdates)
	prometheus.MustRegister(EndpointUpdateErrors)
	prometheus.MustRegister(ServiceCount)
	prometheus.MustRegister(ServiceUpdates)
	prometheus.MustRegister(ServiceUpdateErrors)
}
