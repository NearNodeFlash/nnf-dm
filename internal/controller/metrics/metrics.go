package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	NnfDmDataMovementManagerReconcilesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "nnfdm_datamovementmanager_reconciles_total",
			Help: "Number of total reconciles in nnfdm datamovement manager controller",
		},
	)

	NnfDmDataMovementReconcilesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "nnfdm_datamovement_reconciles_total",
			Help: "Number of total reconciles in nnfdm datamovement controller",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(NnfDmDataMovementManagerReconcilesTotal)
	metrics.Registry.MustRegister(NnfDmDataMovementReconcilesTotal)
}
