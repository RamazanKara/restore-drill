# Quick start

This walks you from a clone to a passing drill in a couple of minutes using the
self-contained Redis demo (no external backups required).

## 1. Build and run the demo

```bash
make build
./bin/restore-drill run --config examples/demo-redis-aof.yaml --runtime docker
```

The demo restores a tiny Redis AOF backup into a disposable `redis:7-alpine`
container, runs validation checks, and prints a pass/fail evidence table.

## 2. Check your environment

Before running your own drills, confirm the runtime and paths are ready:

```bash
restore-drill doctor --config examples/demo-redis-aof.yaml --runtime docker
```

## 3. Write and validate your own config

Start from an example under [`examples/`](https://github.com/RamazanKara/restore-drill/tree/main/examples),
adjust it for your backup, then validate before running:

```bash
restore-drill validate --config drill.yaml
restore-drill run --config drill.yaml --runtime docker
```

Run drills concurrently and emit machine-readable output:

```bash
restore-drill run --config drill.yaml --runtime docker --parallel --format json
```

## 4. Review evidence

```bash
restore-drill status                                  # last run, as a table
restore-drill report --last 90 --output evidence.html # aggregated evidence
```

## Next steps

- [Configuration reference](../guides/configuration.md) — every drill field.
- [Reporting & alerts](../reference/reporting.md) — JSON, HTML, webhooks, Slack, metrics.
- [Kubernetes guide](../guides/kubernetes.md) — schedule drills with the Helm chart.
- [Production rollout](../operations/production.md) — go live safely.
