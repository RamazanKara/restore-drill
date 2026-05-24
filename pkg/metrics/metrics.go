// Package metrics handles Prometheus metric registration and pushing.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const namespace = "restore_drill"

var (
	DrillDuration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "duration_seconds",
			Help:      "Time from start to validated restore.",
		},
		[]string{"drill", "provider", "environment"},
	)

	BackupAge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "backup_age_seconds",
			Help:      "Age of the most recent backup used.",
		},
		[]string{"drill", "provider", "environment"},
	)

	ValidationPassed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "validation_passed",
			Help:      "1 if all checks passed, 0 otherwise.",
		},
		[]string{"drill", "provider", "environment"},
	)

	ChecksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "validation_checks_total",
			Help:      "Number of validation checks reported in the current push.",
		},
		[]string{"drill", "provider", "environment"},
	)

	ChecksFailed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "validation_checks_failed",
			Help:      "Number of validation checks failed in the current push.",
		},
		[]string{"drill", "provider", "environment"},
	)

	LastSuccess = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "last_success_timestamp",
			Help:      "Unix timestamp of last successful drill.",
		},
		[]string{"drill", "provider", "environment"},
	)

	RunsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "runs_total",
			Help:      "Drill executions reported in the current push.",
		},
		[]string{"drill", "provider", "environment", "status"},
	)
)

func init() {
	prometheus.MustRegister(
		DrillDuration,
		BackupAge,
		ValidationPassed,
		ChecksTotal,
		ChecksFailed,
		LastSuccess,
		RunsTotal,
	)
}
