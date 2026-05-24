package metrics

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

// PushResults pushes drill metrics to a Prometheus Pushgateway.
func PushResults(results []engine.DrillResult, pushgatewayURL string, labels map[string]string) error {
	env := labels["environment"]
	if env == "" {
		env = "default"
	}

	registry := prometheus.NewRegistry()
	drillDuration := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: namespace, Name: "duration_seconds", Help: "Time from start to validated restore."},
		[]string{"drill", "provider", "environment"},
	)
	backupAge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: namespace, Name: "backup_age_seconds", Help: "Age of the most recent backup used."},
		[]string{"drill", "provider", "environment"},
	)
	validationPassed := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: namespace, Name: "validation_passed", Help: "1 if all checks passed, 0 otherwise."},
		[]string{"drill", "provider", "environment"},
	)
	checksTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: namespace, Name: "validation_checks_total", Help: "Number of validation checks reported in the current push."},
		[]string{"drill", "provider", "environment"},
	)
	checksFailed := prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: namespace, Name: "validation_checks_failed", Help: "Number of validation checks failed in the current push."},
		[]string{"drill", "provider", "environment"},
	)
	lastSuccess := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: namespace, Name: "last_success_timestamp", Help: "Unix timestamp of last successful drill."},
		[]string{"drill", "provider", "environment"},
	)
	runsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: namespace, Name: "runs_total", Help: "Drill executions reported in the current push."},
		[]string{"drill", "provider", "environment", "status"},
	)
	registry.MustRegister(drillDuration, backupAge, validationPassed, checksTotal, checksFailed, lastSuccess, runsTotal)

	pusher := push.New(pushgatewayURL, "restore_drill").
		Gatherer(registry)

	// Add custom labels as grouping
	for k, v := range labels {
		if k != "environment" {
			pusher = pusher.Grouping(k, v)
		}
	}

	for _, r := range results {
		status := "success"
		if r.Error != nil || !r.ValidationPassed {
			status = "failure"
		}

		drillDuration.With(prometheus.Labels{
			"drill": r.Name, "provider": r.Provider, "environment": env,
		}).Set(r.Duration.Seconds())

		if !r.BackupTimestamp.IsZero() {
			backupAge.With(prometheus.Labels{
				"drill": r.Name, "provider": r.Provider, "environment": env,
			}).Set(time.Since(r.BackupTimestamp).Seconds())
		}

		passed := float64(0)
		if r.ValidationPassed {
			passed = 1
		}
		validationPassed.With(prometheus.Labels{
			"drill": r.Name, "provider": r.Provider, "environment": env,
		}).Set(passed)

		checksTotal.With(prometheus.Labels{
			"drill": r.Name, "provider": r.Provider, "environment": env,
		}).Add(float64(len(r.Checks)))

		failed := 0
		for _, c := range r.Checks {
			if !c.Passed {
				failed++
			}
		}
		checksFailed.With(prometheus.Labels{
			"drill": r.Name, "provider": r.Provider, "environment": env,
		}).Add(float64(failed))

		if r.ValidationPassed && r.Error == nil {
			lastSuccess.With(prometheus.Labels{
				"drill": r.Name, "provider": r.Provider, "environment": env,
			}).Set(float64(time.Now().Unix()))
		}

		runsTotal.With(prometheus.Labels{
			"drill": r.Name, "provider": r.Provider, "environment": env, "status": status,
		}).Inc()
	}

	if err := pusher.Push(); err != nil {
		return fmt.Errorf("push metrics: %w", err)
	}

	slog.Info("metrics pushed", "url", pushgatewayURL, "drills", len(results))
	return nil
}
