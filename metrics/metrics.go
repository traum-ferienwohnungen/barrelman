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
)

func init() {
	// Register prometheus metrics
	prometheus.MustRegister(NodeCount)
	prometheus.MustRegister(EndpointUpdates)
	prometheus.MustRegister(EndpointUpdateErrors)
}
