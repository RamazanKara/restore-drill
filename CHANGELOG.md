# Changelog

All notable changes to restore-drill will be documented in this file.

The format is based on Keep a Changelog, and this project uses semantic
versioning.

## [Unreleased]

### Added

- Published a documentation site (MkDocs Material, deployed to GitHub Pages) with
  a reorganized `docs/` tree (getting-started / guides / reference / operations /
  project) and new CLI reference, troubleshooting, incident-response, and
  state & history pages.

### Changed

- Restructured the Go source under `internal/` — a dedicated `internal/cli`
  wiring layer (thin `main`) and a `config` package split out of `engine`. This
  is internal only: the `go install` path and all CLI, config, JSON, metrics, and
  Helm contracts are unchanged.

## [1.3.0] - 2026-06-28

### Added

- Added an `etcd` provider that restores `etcdctl snapshot` backups into a
  disposable target, starts etcd against the restored data directory, and
  validates the keyspace. New `key_count` (prefix or whole keyspace), `key_get`,
  and `query` checks back etcd validation, with a new `key` check field and a
  Docker integration test that generates and restores a real snapshot.
- Added a `slack` alert type that posts a formatted pass/fail summary to
  Slack-compatible incoming webhooks (Slack and Mattermost).
- Added an `on` condition (`always` default, or `failure`) to `webhook` and
  `slack` alerts so drills can notify only when a restore or validation fails.

### Changed

- Consolidated webhook and Slack delivery onto a shared HTTP retry helper.

## [1.2.1] - 2026-06-28

### Added

- Added unit tests for the version, logging, and target-command helper packages
  (previously untested) plus the report-rendering, webhook-alert, and writable
  directory helpers, raising overall statement coverage.
- Added a `make cover` target that writes `coverage.out` and `coverage.html`,
  and uploaded the coverage report as a CI artifact on every run.
- Added `CITATION.cff` so the project can be cited with structured metadata, and
  a `FUNDING.yml` sponsor configuration.

## [1.2.0] - 2026-06-27

### Added

- Added `restore-drill doctor` for config, runtime, state/report path, release
  tooling, and vulnerability-scan readiness checks.
- Added v1 JSON schemas for drill configuration and run JSON output, with tests
  against examples and generated reporter output.
- Added govulncheck CI gating, a reviewed Docker/Moby vulnerability allowlist,
  CodeQL, OpenSSF Scorecard, dependency review, Dependabot, Cosign release
  signing, and checksum provenance attestation workflow coverage.
- Added maintainer and governance documentation plus richer issue and pull
  request templates.

### Changed

- Updated the Go toolchain line to `go1.25.11` and updated vulnerable
  `golang.org/x/*` modules.
- Changed Docker backup staging to stream tar data through exec instead of the
  Docker archive upload API.

## [1.1.0] - 2026-06-24

### Changed

- Split the CLI entrypoint into focused command, runtime, and reporting modules
  while preserving v1 CLI flags and output contracts.
- Refactored documentation navigation, roadmap naming, local demo guidance, and
  CI/CD examples for readability and consistency with the current workflows.
- Consolidated repeated provider command detection, command selection, backup
  path, and shell quoting helpers.
- Replaced hardcoded compliance-style report language with restore evidence
  checks and neutral evidence report wording.
- Updated Docker SDK, Docker connections, Prometheus, Cobra, AWS SDK modules,
  and Kubernetes modules while keeping the Go 1.25 toolchain line.
- Updated Kubernetes smoke CI to kind `v0.32.0` with
  `kindest/node:v1.36.1`; local smoke checks now report kind/cgroup
  prerequisites explicitly.

### Removed

- Removed unused runtime/reporting internals, including unused restore result
  fields, runtime log streaming, dead Docker readiness helper, unused default
  ports, unused Prometheus collector globals, and the unused JSON file reporter.
- Removed per-drill Prometheus alert examples. Deprecated per-drill
  `prometheus` alerts remain accepted as no-ops for v1 config compatibility.

## [1.0.1] - 2026-05-25

### Added

- Added configured run report artifacts: `reporting.format: [json, html]` with
  `reporting.output` now writes per-run JSON and evidence HTML files.
- Added webhook alert headers for authenticated delivery.
- Added reporting documentation covering JSON compatibility, webhook payloads,
  and Prometheus alert examples.
- Added a documentation index and refactored README, configuration, Kubernetes,
  production, release, CI/CD, architecture, and roadmap docs around the current
  v1 scope.
- Added support, upgrade, and deprecation policy documentation.

### Changed

- Hardened webhook retries with context-aware waits and explicit retry coverage.
- Extended state history tests for malformed history entries.
- Surfaced provider cleanup and runtime destroy failures in drill results.
- Made stdout reports show top-level drill errors even when no validation checks
  were produced.
- Extended negative-path staging tests for local copy diagnostics and empty S3
  prefixes.

## [1.0.0] - 2026-05-24

### Changed

- Removed restore cost estimation from the roadmap to keep restore-drill focused
  on restore verification.
- Clarified v1.0 release readiness, project non-goals, and Pushgateway-based
  Kubernetes metrics guidance.
- Changed Pushgateway writes to replace the current grouping on each run,
  avoiding stale counter accumulation between CronJob executions.
- Added JSON reporter shape coverage for retained targets, check evidence,
  backup timing, and failures.
- Made latest-run and history persistence atomic, with concurrent same-timestamp
  history appends preserved as separate entries.
- Added provider-specific check validation and preflight coverage for required
  restore-image tools.
- Added per-check failure evidence to evidence reports.
- Added lifecycle regression tests and Kubernetes restore-target pod
  customization for service accounts, pull secrets, labels, and annotations.
- Made the Helm chart fail fast when neither inline config nor an existing
  ConfigMap is provided, and expanded Helm rendering checks for Kubernetes
  runtime options.
- Expanded Docker integration coverage to restore compressed PostgreSQL/MySQL
  logical dumps and Redis AOF/RDB fixtures with real key/value validation.
- Added provider restore-path regression tests for pgBackRest PITR, WAL-G PITR
  recovery config, `pg_restore`, xtrabackup, and mariabackup command
  construction.
- Added a Docker image smoke target and CI job to prove the release image boots
  and reports its version.
- Added archive materialization for pgBackRest, xtrabackup, and mariabackup
  physical restore sources, including preflight checks for required archive
  tools.
- Added S3-compatible staging coverage for latest-object prefix resolution and
  target staging.
- Added real Docker integration fixtures for PostgreSQL pgBackRest,
  PostgreSQL WAL-G, Percona xtrabackup, and MariaDB mariabackup physical
  restores.
- Added WAL-G local repository staging with generated recovery config for staged
  file-backed repositories.
- Added support for MariaDB-native client/admin/safe binary names.

## [0.2.0] - 2026-05-23

### Added

- Release-hardening for the next stable open-source release.
- Explicit Docker/Kubernetes runtime selection.
- Shared local and S3-compatible backup staging.
- Open-source governance and release hygiene files.
- Kubernetes smoke testing, generated Docker integration fixtures, GoReleaser
  SBOMs, and CI release gates.

### Changed

- Standardized project identity on `github.com/RamazanKara/restore-drill` and
  `ghcr.io/ramazankara/restore-drill`.
- Tightened configuration validation for provider/tool/repo/check compatibility.

## [0.1.0] - 2026-05-22

Initial public release.
