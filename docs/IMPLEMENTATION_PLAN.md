# v1.0 Release Readiness

restore-drill should reach v1.0 when it is excellent at one job: proving that real backups can be restored into disposable environments and validated with useful evidence.

## Product focus

restore-drill is focused on restore verification:

- create an isolated restore target
- stage backup artifacts from documented sources
- restore with the configured provider/tool
- validate the restored data
- report RTO/RPO, validation evidence, retained-target details, and history
- publish metrics and webhook notifications for operational follow-up

Non-goals:

- backup scheduling or orchestration
- backup repository management
- infrastructure inventory
- cost estimation
- general-purpose database migration
- broad provider sprawl before existing providers are boringly reliable

## Current stable scope

- Canonical module: `github.com/RamazanKara/restore-drill`
- License: Apache-2.0
- Container image: `ghcr.io/ramazankara/restore-drill`
- Supported providers: PostgreSQL, MySQL/MariaDB, Redis
- Supported runtimes: Docker and Kubernetes
- Supported outputs: stdout table, JSON, HTML compliance report, webhook, Prometheus Pushgateway

## v1.0 release gates

- `make build`
- `make vet`
- `make test-unit`
- `make lint`
- `make check-examples`
- `make helm-lint` with a real example config and Kubernetes runtime option render coverage
- `goreleaser check`
- `make docker-smoke`
- `goreleaser release --snapshot --clean --skip=publish`
- `RESTORE_DRILL_INTEGRATION=1 go test -race -count=1 -timeout=20m ./test/integration/...`
- `make test-k8s`

The v1.0 tag is blocked unless CI proves every README-supported provider, runtime, example, Helm template, report format, and release artifact end to end.

## v1.0 hardening backlog

- Keep Docker integration coverage for generated logical fixtures and real physical/PITR flows: pgBackRest, WAL-G, xtrabackup, and mariabackup.
- Keep physical backup archive staging for `.tar`, `.tar.gz`, `.tgz`, `.xbstream`, and `.xbstream.gz` covered for pgBackRest, xtrabackup, and mariabackup paths.
- Keep generated Docker fixture coverage for compressed `pg_dump`, compressed `mysqldump`, Redis AOF, and Redis RDB green on every release.
- Keep provider restore-path unit coverage for pgBackRest PITR, WAL-G PITR recovery config, `pg_restore`, xtrabackup, and mariabackup command construction.
- Keep S3-compatible staging coverage for exact object downloads and latest-object prefix resolution.
- Extend negative-path tests for missing credentials, staging failures, provider restore failures, and cleanup failures.
- Add Kubernetes integration coverage for retained pods, namespace overrides, target pod service accounts, image pull secrets, resource settings, network policy, and failure cleanup.
- Document JSON report compatibility guarantees and keep the current golden shape test updated.
- Extend HTML report golden coverage around RTO/RPO summaries and compliance control rendering.
- Confirm Pushgateway metrics are idempotent across repeated runs and document Prometheus alert examples.
- Extend state/history durability tests around interrupted writes and malformed history entries.
- Document upgrade policy, deprecation policy, and support windows.
- Add release notes that clearly separate GA behavior from roadmap candidates.

## Roadmap candidates after v1.0

- Standalone object-store restore drills
- etcd snapshot restore drills
- ClickHouse and MongoDB providers
- Velero restore validation
- PITR fuzzing
- Multi-region restore drills
