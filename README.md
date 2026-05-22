# restore-drill

**Automated backup verification for self-hosted infrastructure.**

Backups that are never restored are not backups. `restore-drill` continuously proves your recovery works by restoring real backups into ephemeral environments, running validation queries, and publishing RTO/RPO as Prometheus metrics.

[![CI](https://github.com/RamazanKara/restore-drill/actions/workflows/ci.yml/badge.svg)](https://github.com/RamazanKara/restore-drill/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/fluentorbit/restore-drill)](https://goreportcard.com/report/github.com/fluentorbit/restore-drill)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/RamazanKara/restore-drill)](https://github.com/RamazanKara/restore-drill/releases)

## The problem

Every team has backups. Almost nobody verifies them regularly.

- pgBackRest runs nightly — but when was the last time you restored from it?
- Velero snapshots exist — but can they actually reconstruct your database?
- Your RTO is "4 hours" in the DR doc — is that a measurement or a guess?

Compliance frameworks (NIS2, ISO 27001, BSI C5) require **tested** recovery. Not documented. Tested.

## What restore-drill does

```
┌─────────────┐     ┌──────────────┐     ┌────────────────┐     ┌──────────────┐
│ Backup Store│────▶│ Restore Job  │────▶│ Validation     │────▶│ Metrics +    │
│ (S3, local) │     │ (ephemeral)  │     │ (queries/checks)│     │ Report       │
└─────────────┘     └──────────────┘     └────────────────┘     └──────────────┘
```

1. **Restore** — Spins up an ephemeral container, pulls the latest backup, restores it
2. **Validate** — Runs configurable checks: row counts, schema integrity, query correctness, data freshness
3. **Measure** — Records restore duration (RTO), backup age (RPO), and validation results
4. **Report** — Publishes Prometheus metrics, sends alerts on failure, generates compliance reports

## Supported backends

| Backend | Backup tool | Status |
|---------|------------|--------|
| PostgreSQL | pgBackRest, pg_dump, WAL-G | GA |
| MySQL/MariaDB | mysqldump, xtrabackup, mariabackup | GA |
| Redis | RDB snapshots, AOF | GA |
| S3-compatible | rclone, mc (MinIO client) | Planned |
| etcd | etcdctl snapshot | Planned |
| ClickHouse | clickhouse-backup | Planned |
| MongoDB | mongodump | Planned |

## Quick start

### CLI

```bash
# Install
go install github.com/fluentorbit/restore-drill/cmd/restore-drill@latest

# Run a drill against your PostgreSQL backup
restore-drill run --config drill.yaml

# Check last drill results
restore-drill status
```

### Kubernetes CronJob

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: restore-drill-postgres
spec:
  schedule: "0 3 * * 1"    # Weekly, Monday 3am
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: Never
          containers:
            - name: drill
              image: ghcr.io/ramazankara/restore-drill:latest
              args: ["run", "--config", "/etc/drill/config.yaml"]
              volumeMounts:
                - name: config
                  mountPath: /etc/drill
          volumes:
            - name: config
              configMap:
                name: restore-drill-config
```

## Configuration

```yaml
# drill.yaml
drills:
  - name: production-postgres
    provider: postgres
    schedule: "0 3 * * 1"          # When to run (cron, or "manual")
    backup:
      tool: pgbackrest
      stanza: main
      repo:
        type: s3
        bucket: my-backups
        endpoint: s3.eu-central-1.amazonaws.com
        prefix: pgbackrest/
    restore:
      target: latest                # or specific PITR timestamp
      container:
        image: postgres:16
        resources:
          memory: 2Gi
          cpu: "1"
      timeout: 30m
    checks:
      - name: user-count
        type: query
        sql: "SELECT count(*) FROM users"
        expect: "> 0"
      - name: data-freshness
        type: query
        sql: "SELECT max(updated_at) FROM orders"
        expect: "age < 25h"         # Data freshness check
      - name: schema-version
        type: schema
        sql: "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1"
        expect: ">= 142"
    alerts:
      - type: prometheus            # Push to Pushgateway
        endpoint: http://pushgateway:9091
      - type: webhook
        url: https://hooks.slack.com/...

  - name: production-redis
    provider: redis
    backup:
      tool: rdb
      source:
        type: s3
        bucket: my-backups
        prefix: redis/
    restore:
      container:
        image: redis:7-alpine
      timeout: 5m
    checks:
      - name: key-count
        type: key_count
        expect: "> 1000"
      - name: session-keys
        type: key_sample
        keys: ["session:*", "cache:*"]
        expect: "> 0"

metrics:
  prometheus:
    enabled: true
    pushgateway: http://pushgateway:9091
    labels:
      environment: production
      team: platform

reporting:
  format: [json, html]
  output: ./reports/
  retention: 90d
```

## Metrics

All metrics are prefixed with `restore_drill_`.

| Metric | Type | Description |
|--------|------|-------------|
| `restore_drill_duration_seconds` | Gauge | Time from start to validated restore |
| `restore_drill_backup_age_seconds` | Gauge | Age of the most recent backup used |
| `restore_drill_validation_passed` | Gauge | 1 if all checks passed, 0 otherwise |
| `restore_drill_validation_checks_total` | Counter | Number of validation checks executed |
| `restore_drill_validation_checks_failed` | Counter | Number of validation checks failed |
| `restore_drill_last_success_timestamp` | Gauge | Unix timestamp of last successful drill |
| `restore_drill_runs_total` | Counter | Total drill executions (label: status) |

### Example alert rules

```yaml
groups:
  - name: restore-drill
    rules:
      - alert: RestoreDrillFailed
        expr: restore_drill_validation_passed == 0
        for: 0m
        labels:
          severity: critical
        annotations:
          summary: "Restore drill failed for {{ $labels.drill }}"

      - alert: RestoreDrillStale
        expr: time() - restore_drill_last_success_timestamp > 7 * 86400
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "No successful restore drill in 7 days for {{ $labels.drill }}"

      - alert: RestoreDrillSlow
        expr: restore_drill_duration_seconds > 1800
        for: 0m
        labels:
          severity: warning
        annotations:
          summary: "Restore drill took > 30min for {{ $labels.drill }}"
```

## Compliance output

Generate audit-ready reports:

```bash
restore-drill report --format html --last 90d --output compliance-q2.html
```

The report includes:
- Drill execution history with timestamps
- RTO measurements per drill
- RPO measurements (backup freshness at time of test)
- Pass/fail status for each validation check
- Trend charts (RTO over time)

Suitable for: NIS2 Art. 21, ISO 27001 A.12.3, BSI C5 OPS-04.

## Architecture

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full component design.

## Helm chart

```bash
helm install restore-drill deploy/helm/ \
  --namespace restore-drill \
  --create-namespace \
  --set-file config=drill.yaml
```

See [deploy/helm/](deploy/helm/) for values reference.

## Roadmap

- [x] PostgreSQL provider (pgBackRest, pg_dump, WAL-G)
- [x] MySQL provider (mysqldump, xtrabackup, mariabackup)
- [x] Redis provider (RDB, AOF)
- [x] Prometheus metrics + Pushgateway
- [x] JSON output format
- [x] Helm chart (CronJob)
- [ ] S3 provider (rclone)
- [ ] etcd provider (etcdctl)
- [ ] ClickHouse provider
- [ ] MongoDB provider
- [ ] Velero integration (restore from Velero snapshots)
- [ ] PITR fuzzing (restore to random point, validate consistency)
- [ ] Multi-region drill (restore in a different region)
- [ ] Cost estimation (ephemeral compute cost per drill)

## Contributing

Contributions welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

```bash
git clone https://github.com/fluentorbit/restore-drill.git
cd restore-drill
make build
make test
make lint
```

## License

Apache License 2.0 — see [LICENSE](LICENSE).

---

Built by [FluentOrbit](https://fluentorbit.de) — platform engineering for self-hosted infrastructure.
