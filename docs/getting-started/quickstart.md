# Quick start

The Redis demo includes a small backup fixture. It requires a working Docker
daemon and permission to pull and run `redis:7-alpine`.

## 1. Build and run the demo

```bash
make build
export PATH="$PWD/bin:$PATH"
restore-drill validate --config examples/demo-redis-aof.yaml
restore-drill run --config examples/demo-redis-aof.yaml --runtime docker
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
copy the chosen file to `drill.yaml`, and adjust it for your backup before running:

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
