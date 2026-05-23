# Open Source Release Checklist

This checklist tracks the implementation bar for the first public release.

## v0.2.0 scope

- Canonical module: `github.com/RamazanKara/restore-drill`
- License: Apache-2.0
- Container image: `ghcr.io/ramazankara/restore-drill`
- Supported providers: PostgreSQL, MySQL/MariaDB, Redis
- Supported runtimes: Docker and Kubernetes
- Supported outputs: stdout table, JSON, HTML compliance report, webhook, Prometheus Pushgateway

## Release gates

- `make build`
- `make vet`
- `make test-unit`
- `make lint`
- `make check-examples`
- `helm lint deploy/helm/restore-drill`
- `helm template restore-drill deploy/helm/restore-drill`
- `goreleaser check`
- `goreleaser release --snapshot --clean --skip=publish`
- `RESTORE_DRILL_INTEGRATION=1 go test -race -count=1 -timeout=10m ./test/integration/...`

## Roadmap after v0.2.0

- Standalone object-store drills
- etcd snapshot restore drills
- ClickHouse and MongoDB providers
- Velero restore validation
- PITR fuzzing
- Multi-region restore drills
- Restore cost estimation
