# restore-drill

**Automated backup restore verification for self-hosted infrastructure.**

Backups that are never restored are guesses. `restore-drill` restores real
backup artifacts into disposable Docker containers or Kubernetes pods, validates
the restored data, records RTO/RPO evidence, and publishes machine-readable
results for audits and alerts.

[![CI](https://github.com/RamazanKara/restore-drill/actions/workflows/ci.yml/badge.svg)](https://github.com/RamazanKara/restore-drill/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/RamazanKara/restore-drill)](https://goreportcard.com/report/github.com/RamazanKara/restore-drill)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/RamazanKara/restore-drill)](https://github.com/RamazanKara/restore-drill/releases)

## See It Run

![restore-drill Redis restore demo](docs/assets/restore-drill-demo.gif)

The recording runs a real Redis AOF restore in Docker and is generated from
[docs/assets/restore-drill-demo.tape](docs/assets/restore-drill-demo.tape).

## What it does

1. Creates an isolated restore target with Docker or Kubernetes.
2. Stages local or S3-compatible backup artifacts when needed.
3. Runs the configured provider restore flow.
4. Executes validation checks against restored data.
5. Writes local history, JSON/HTML evidence, webhooks, and Prometheus
   Pushgateway metrics.

restore-drill is intentionally focused on one job: proving that backups can be
restored. It does not schedule backups, manage repositories, estimate costs,
inventory infrastructure, or replace observability platforms.

## Supported in v1

| Area | Supported surface |
| --- | --- |
| Providers | PostgreSQL, MySQL/MariaDB, Redis |
| Backup tools | `pg_dump`, `pg_restore`, `pgbackrest`, `wal-g`/`walg`, `mysqldump`, `xtrabackup`, `mariabackup`, Redis RDB, Redis AOF |
| Backup sources | Local files/directories, mounted target paths, S3-compatible objects and prefixes |
| Runtimes | Docker and Kubernetes |
| Outputs | stdout table, run JSON, HTML evidence reports, webhooks, local history, Prometheus Pushgateway |
| Kubernetes | Helm CronJob, namespace-scoped RBAC, restore pod labels/annotations, image pull secrets, resources, NetworkPolicy |

Provider restore images must include the database runtime, client tools, and the
selected backup tool. Preflight checks fail early when required commands are
missing.

Future candidates are tracked in [docs/ROADMAP.md](docs/ROADMAP.md). Cost
estimation is a non-goal.

## Install

```bash
go install github.com/RamazanKara/restore-drill/cmd/restore-drill@latest
```

Release binaries and container images are published from tags:

```bash
docker pull ghcr.io/ramazankara/restore-drill:latest
```

## Quick start

Run the self-contained Redis demo from a clone:

```bash
make build
./bin/restore-drill run --config examples/demo-redis-aof.yaml --runtime docker
```

Then validate and run your own drill config:

```bash
restore-drill validate --config examples/drill.yaml
restore-drill run --config drill.yaml --runtime docker
restore-drill run --config drill.yaml --runtime docker --parallel --format json
restore-drill status
restore-drill report --last 90 --output restore-evidence.html
```

Incident mode keeps the restore target available for inspection:

```bash
restore-drill run \
  --config drill.yaml \
  --target "2026-05-20T14:30:00Z" \
  --no-cleanup
```

When `--no-cleanup` is set, stdout, JSON, and state include the retained
container or pod ID, host, and port map.

## Minimal config

```yaml
drills:
  - name: production-postgres
    provider: postgres
    backup:
      tool: pg_dump
      source: /backups/postgres/latest.sql.gz
    restore:
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
    alerts:
      - type: webhook
        url: ${RESTORE_DRILL_WEBHOOK_URL}
        headers:
          Authorization: "Bearer ${RESTORE_DRILL_WEBHOOK_TOKEN}"

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

## Kubernetes

```bash
helm install restore-drill deploy/helm/restore-drill \
  --namespace restore-drill \
  --create-namespace \
  --set-file config.inline=drill.yaml
```

The chart runs `restore-drill run --runtime=kubernetes`, creates
namespace-scoped RBAC for ephemeral restore pods, and supports inline or
ConfigMap-backed drill configuration with Secret-driven environment variables.

## Documentation

- [docs/README.md](docs/README.md): documentation map
- [docs/CONFIGURATION.md](docs/CONFIGURATION.md): full YAML reference
- [docs/KUBERNETES.md](docs/KUBERNETES.md): Helm and Kubernetes runtime guide
- [docs/REPORTING.md](docs/REPORTING.md): JSON, HTML, webhook, and metrics contracts
- [docs/PRODUCTION.md](docs/PRODUCTION.md): production rollout checklist
- [docs/ci-integration.md](docs/ci-integration.md): CI/CD and scheduled drill examples
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): engine, runtime, provider, and state model
- [docs/RELEASE.md](docs/RELEASE.md): release process
- [docs/SUPPORT.md](docs/SUPPORT.md): support, upgrade, and deprecation policy
- [docs/ROADMAP.md](docs/ROADMAP.md): stable scope, non-goals, and roadmap candidates

## Development

```bash
make build
make test-unit
make lint
make check-examples
```

`make verify` also runs Helm and GoReleaser checks and requires those tools on
`PATH`. Docker integration tests are opt-in because they create real containers:

```bash
make test-integration
```

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
