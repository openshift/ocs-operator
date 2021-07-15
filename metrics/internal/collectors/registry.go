package collectors

import (
	"github.com/openshift/ocs-operator/metrics/internal/options"
	"github.com/prometheus/client_golang/prometheus"
)

// RegisterCustomResourceCollectors registers the custom resource collectors
// in the given prometheus.Registry
// This is used to expose metrics about the Custom Resources
func RegisterCustomResourceCollectors(registry *prometheus.Registry, opts *options.Options) {
	cephObjectStoreCollector := NewCephObjectStoreCollector(opts)
	cephObjectStoreCollector.Run(opts.StopCh)
	OBMetricsCollector := NewOBCollector(opts)
	OBMetricsCollector.Run(opts.StopCh)
	registry.MustRegister(
		cephObjectStoreCollector,
		OBMetricsCollector,
	)
}
