package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	NnfDmDataMovementReconcilesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "nnfdm_datamovement_reconciles_total",
			Help: "Number of total reconciles in nnfdm datamovement controller",
		},
	)

	NnfDmRsyncNodeDataMovementReconcilesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "nnfdm_rsyncnode_datamovement_reconciles_total",
			Help: "Number of total reconciles in nnfdm rsyncnode_datamovement controller",
		},
	)

	NnfDmRsyncTemplateReconcilesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "nnfdm_rsynctemplate_reconciles_total",
			Help: "Number of total reconciles in nnfdm rsynctemplate controller",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(NnfDmDataMovementReconcilesTotal)
	metrics.Registry.MustRegister(NnfDmRsyncNodeDataMovementReconcilesTotal)
	metrics.Registry.MustRegister(NnfDmRsyncTemplateReconcilesTotal)
}
