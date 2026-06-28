# Roadmap and Release Readiness

restore-drill is stable when it is excellent at one job: proving that real
backups can be restored into disposable environments and validated with useful
evidence.

This page is deliberately narrow. It explains what is stable today, what must
stay true for releases, and which ideas are future candidates rather than
current product promises.

## Product Focus

restore-drill verifies restores:

- create an isolated restore target
- stage backup artifacts from documented sources
- restore with the configured provider and tool
- validate restored data
- report RTO/RPO, validation evidence, retained-target details, and history
- publish metrics and webhook notifications for operational follow-up

It intentionally does not:

- backup scheduling or orchestration
- backup repository management
- infrastructure inventory
- cost estimation
- general-purpose database migration
- broad provider sprawl before existing providers are boringly reliable

## Stable v1 Scope

- Canonical module: `github.com/RamazanKara/restore-drill`
- License: Apache-2.0
- Container image: `ghcr.io/ramazankara/restore-drill`
- Supported providers: PostgreSQL, MySQL/MariaDB, Redis, etcd
- Supported runtimes: Docker and Kubernetes
- Supported outputs: stdout table, run JSON, HTML evidence report, webhook,
  Slack/Mattermost alerts, local history, Prometheus Pushgateway

The public contracts that must remain compatible within v1 are documented in
[SUPPORT.md](SUPPORT.md) and [REPORTING.md](REPORTING.md).

## Release Gates

Every release should pass:

- `make build`
- `make vet`
- `make test-unit`
- `make lint`
- `make check-examples`
- `make helm-lint`
- `goreleaser check`
- `make docker-smoke`
- `goreleaser release --snapshot --clean --skip=publish`
- `make test-integration`
- `make test-k8s`

The release is not ready if README-supported provider, runtime, example, Helm,
reporting, or release artifact behavior is not proven by CI or local release
checks.

## Already Hardened

- Standardized project identity on `github.com/RamazanKara/restore-drill` and
  `ghcr.io/ramazankara/restore-drill`.
- Added explicit Docker/Kubernetes runtime selection.
- Added shared local and S3-compatible backup staging.
- Added provider capability preflight checks.
- Added archive materialization for pgBackRest, WAL-G, xtrabackup, and
  mariabackup restore paths.
- Added Docker integration fixtures for compressed PostgreSQL/MySQL logical
  dumps, pgBackRest, WAL-G, xtrabackup, mariabackup, Redis AOF, and Redis RDB.
- Added Kubernetes smoke coverage for pod lifecycle, retained pods, and Helm
  runtime options.
- Added JSON report shape coverage and documented the v1 compatibility contract.
- Added configured run report artifacts through `reporting.format` and
  `reporting.output`.
- Added per-check failure evidence to HTML evidence reports.
- Added webhook headers and context-aware retry behavior.
- Confirmed Pushgateway replacement behavior for repeated CronJob runs.
- Made latest-run and history persistence atomic.
- Added state history coverage for malformed entries.
- Added cleanup failure surfacing and negative-path staging coverage.
- Documented support windows, stable contracts, upgrades, and deprecations.

## Maintenance Priorities

- Keep Docker integration coverage green for generated logical fixtures,
  physical restore flows, and PITR-capable providers.
- Keep archive staging coverage green for `.tar`, `.tar.gz`, `.tgz`,
  `.xbstream`, and `.xbstream.gz`.
- Keep adding negative-path coverage for missing credentials, staging failures,
  provider restore failures, cleanup failures, and retained-target handling.
- Broaden Kubernetes integration coverage for namespace overrides, target pod
  service accounts, image pull secrets, resource settings, NetworkPolicy, and
  failure cleanup.
- Move GoReleaser Docker configuration to `dockers_v2` when the replacement is
  no longer experimental for the project's release needs.

## Future Candidates

These are future candidates, not current GA claims:

- standalone object-store restore drills
- ClickHouse and MongoDB providers
- Velero restore validation
- PITR fuzzing
- multi-region restore drills
- additional chat/alert destinations (PagerDuty, Microsoft Teams, Discord)
