package metrics

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"

	"github.com/fluentorbit/restore-drill/pkg/engine"
)

// PushResults pushes drill metrics to a Prometheus Pushgateway.
func PushResults(results []engine.DrillResult, pushgatewayURL string, labels map[string]string) error {
	env := labels["environment"]
	if env == "" {
		env = "default"
	}

	pusher := push.New(pushgatewayURL, "restore_drill").
		Collector(DrillDuration).
		Collector(BackupAge).
		Collector(ValidationPassed).
		Collector(ChecksTotal).
		Collector(ChecksFailed).
		Collector(LastSuccess).
		Collector(RunsTotal)

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

		DrillDuration.With(prometheus.Labels{
			"drill": r.Name, "provider": r.Provider, "environment": env,
		}).Set(r.Duration.Seconds())

		if !r.BackupTimestamp.IsZero() {
			BackupAge.With(prometheus.Labels{
				"drill": r.Name, "provider": r.Provider, "environment": env,
			}).Set(time.Since(r.BackupTimestamp).Seconds())
		}

		passed := float64(0)
		if r.ValidationPassed {
			passed = 1
		}
		ValidationPassed.With(prometheus.Labels{
			"drill": r.Name, "provider": r.Provider, "environment": env,
		}).Set(passed)

		ChecksTotal.With(prometheus.Labels{
			"drill": r.Name, "provider": r.Provider, "environment": env,
		}).Add(float64(len(r.Checks)))

		failed := 0
		for _, c := range r.Checks {
			if !c.Passed {
				failed++
			}
		}
		ChecksFailed.With(prometheus.Labels{
			"drill": r.Name, "provider": r.Provider, "environment": env,
		}).Add(float64(failed))

		if r.ValidationPassed && r.Error == nil {
			LastSuccess.With(prometheus.Labels{
				"drill": r.Name, "provider": r.Provider, "environment": env,
			}).Set(float64(time.Now().Unix()))
		}

		RunsTotal.With(prometheus.Labels{
			"drill": r.Name, "provider": r.Provider, "environment": env, "status": status,
		}).Inc()
	}

	if err := pusher.Add(); err != nil {
		return fmt.Errorf("push metrics: %w", err)
	}

	slog.Info("metrics pushed", "url", pushgatewayURL, "drills", len(results))
	return nil
}
