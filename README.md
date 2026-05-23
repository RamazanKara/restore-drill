# restore-drill

**Automated backup restore verification for self-hosted infrastructure.**

Backups that are never restored are guesses. `restore-drill` restores real backups into ephemeral Docker containers or Kubernetes pods, runs validation checks, records RTO/RPO evidence, and publishes machine-readable output for audits and alerts.

[![CI](https://github.com/RamazanKara/restore-drill/actions/workflows/ci.yml/badge.svg)](https://github.com/RamazanKara/restore-drill/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/RamazanKara/restore-drill)](https://goreportcard.com/report/github.com/RamazanKara/restore-drill)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/RamazanKara/restore-drill)](https://github.com/RamazanKara/restore-drill/releases)

## What it does

1. Creates an ephemeral restore target with Docker or Kubernetes.
2. Stages a local or S3-compatible backup artifact when needed.
3. Runs the configured provider restore command.
4. Executes validation checks against the restored data.
5. Writes status, history, JSON/HTML reports, webhook notifications, and Prometheus Pushgateway metrics.

## Supported restore paths

| Provider | Backup tools | Runtime status | Release gate coverage | Notes |
| --- | --- | --- | --- | --- |
| PostgreSQL | `pg_dump`, `pg_restore`, `pgbackrest`, `wal-g` | Docker, Kubernetes | Generated Docker fixture covers `pg_dump`. | Restore image must include `psql`, `pg_isready`, and the selected backup tool. |
| MySQL/MariaDB | `mysqldump`, `xtrabackup`, `mariabackup` | Docker, Kubernetes | Generated Docker fixture covers `mysqldump`. | Restore image must include `mysql`, `mysqladmin`, and the selected physical backup tool. |
| Redis | RDB, AOF | Docker, Kubernetes | Generated Docker fixture covers AOF; kind smoke covers AOF on Kubernetes. | Restore image must include `redis-server` and `redis-cli`. |
| Prometheus Pushgateway | metrics push | GA | Enabled through `metrics.prometheus`. |
| JSON/HTML compliance reports | local history | GA | `report` reads `~/.restore-drill/history`. |
| Webhooks | JSON POST | GA | Configure per-drill `alerts` with `type: webhook`. |

Planned providers: standalone S3 object drills, etcd, ClickHouse, MongoDB, Velero, PITR fuzzing, multi-region restore drills, and cost estimation.

## Install

```bash
go install github.com/RamazanKara/restore-drill/cmd/restore-drill@latest
```

Release binaries and container images are published from tags:

```bash
docker pull ghcr.io/ramazankara/restore-drill:latest
```

## Quick start

```bash
restore-drill validate --config examples/drill.yaml
restore-drill run --config drill.yaml --runtime docker
restore-drill run --config drill.yaml --runtime docker --parallel --format json
restore-drill status
restore-drill report --last 90 --output compliance-report.html
```

Incident mode keeps the restore target available for inspection:

```bash
restore-drill run \
  --config drill.yaml \
  --target "2026-05-20T14:30:00Z" \
  --no-cleanup
```

When `--no-cleanup` is set, stdout/JSON/state include the retained container or pod ID, host, and port map.

## Example config

```yaml
drills:
  - name: production-postgres
    provider: postgres
    backup:
      tool: pg_dump
      source: /backups/postgres/latest.sql.gz
    restore:
      target: latest
      timeout: 30m
      container:
        image: postgres:16
        env:
          POSTGRES_HOST_AUTH_METHOD: trust
        resources:
          memory: 2Gi
          cpu: "1"
    checks:
      - name: users-exist
        type: query
        sql: "SELECT count(*) FROM users"
        expect: "> 0"
      - name: required-extensions
        type: extensions
        expect: "pgcrypto, uuid-ossp"
    alerts:
      - type: webhook
        url: https://hooks.example.invalid/restore-drill

metrics:
  prometheus:
    enabled: true
    pushgateway: http://pushgateway:9091
    labels:
      environment: production
      team: platform
```

See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for the full YAML reference.

## Kubernetes

```bash
helm install restore-drill deploy/helm/restore-drill \
  --namespace restore-drill \
  --create-namespace \
  --set-file config.inline=drill.yaml
```

The chart runs the CLI with `--runtime=kubernetes`, creates namespace-scoped RBAC for ephemeral restore pods, and supports ConfigMap/Secret driven configuration. See [docs/KUBERNETES.md](docs/KUBERNETES.md).

For production rollout guidance, see [docs/PRODUCTION.md](docs/PRODUCTION.md).

## Metrics

All metrics are prefixed with `restore_drill_`.

| Metric | Type | Description |
| --- | --- | --- |
| `restore_drill_duration_seconds` | Gauge | Time from start to validated restore. |
| `restore_drill_backup_age_seconds` | Gauge | Age of the backup used. |
| `restore_drill_validation_passed` | Gauge | `1` when all checks passed. |
| `restore_drill_validation_checks_total` | Counter | Checks executed in the current push. |
| `restore_drill_validation_checks_failed` | Counter | Failed checks in the current push. |
| `restore_drill_last_success_timestamp` | Gauge | Unix timestamp of the last successful drill. |
| `restore_drill_runs_total` | Counter | Runs by status. |

## Development

```bash
make build
make test-unit
make lint
make check-examples
```

`make verify` also runs Helm and GoReleaser checks and requires those tools on `PATH`. Docker integration tests are opt-in and cover generated `pg_dump`, `mysqldump`, and Redis AOF fixtures:

```bash
RESTORE_DRILL_INTEGRATION=1 go test -race -count=1 -timeout=10m ./test/integration/...
```

Release details are in [docs/RELEASE.md](docs/RELEASE.md).

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
