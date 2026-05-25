# Changelog

All notable changes to restore-drill will be documented in this file.

The format is based on Keep a Changelog, and this project uses semantic versioning.

## [Unreleased]

## [1.0.1] - 2026-05-25

### Added

- Added configured run report artifacts: `reporting.format: [json, html]` with `reporting.output` now writes per-run JSON and compliance HTML files.
- Added webhook alert headers for authenticated delivery.
- Added reporting documentation covering JSON compatibility, webhook payloads, and Prometheus alert examples.
- Added a documentation index and refactored README, configuration, Kubernetes, production, release, CI/CD, architecture, and roadmap docs around the current v1 scope.
- Added support, upgrade, and deprecation policy documentation.

### Changed

- Hardened webhook retries with context-aware waits and explicit retry coverage.
- Extended state history tests for malformed history entries.
- Surfaced provider cleanup and runtime destroy failures in drill results.
- Made stdout reports show top-level drill errors even when no validation checks were produced.
- Extended negative-path staging tests for local copy diagnostics and empty S3 prefixes.

## [1.0.0] - 2026-05-24

### Changed

- Removed restore cost estimation from the roadmap to keep restore-drill focused on restore verification.
- Clarified v1.0 release readiness, project non-goals, and Pushgateway-based Kubernetes metrics guidance.
- Changed Pushgateway writes to replace the current grouping on each run, avoiding stale counter accumulation between CronJob executions.
- Added JSON reporter shape coverage for retained targets, check evidence, backup timing, and failures.
- Made latest-run and history persistence atomic, with concurrent same-timestamp history appends preserved as separate entries.
- Added provider-specific check validation and preflight coverage for required restore-image tools.
- Added per-check failure evidence to compliance reports.
- Added lifecycle regression tests and Kubernetes restore-target pod customization for service accounts, pull secrets, labels, and annotations.
- Made the Helm chart fail fast when neither inline config nor an existing ConfigMap is provided, and expanded Helm rendering checks for Kubernetes runtime options.
- Expanded Docker integration coverage to restore compressed PostgreSQL/MySQL logical dumps and Redis AOF/RDB fixtures with real key/value validation.
- Added provider restore-path regression tests for pgBackRest PITR, WAL-G PITR recovery config, `pg_restore`, xtrabackup, and mariabackup command construction.
- Added a Docker image smoke target and CI job to prove the release image boots and reports its version.
- Added archive materialization for pgBackRest, xtrabackup, and mariabackup physical restore sources, including preflight checks for required archive tools.
- Added S3-compatible staging coverage for latest-object prefix resolution and target staging.
- Added real Docker integration fixtures for PostgreSQL pgBackRest, PostgreSQL WAL-G, Percona xtrabackup, and MariaDB mariabackup physical restores.
- Added WAL-G local repository staging with generated recovery config for staged file-backed repositories.
- Added support for MariaDB-native client/admin/safe binary names.

## [0.2.0] - 2026-05-23

### Added

- Release-hardening for the next stable open-source release.
- Explicit Docker/Kubernetes runtime selection.
- Shared local and S3-compatible backup staging.
- Open-source governance and release hygiene files.
- Kubernetes smoke testing, generated Docker integration fixtures, GoReleaser SBOMs, and CI release gates.

### Changed

- Standardized project identity on `github.com/RamazanKara/restore-drill` and `ghcr.io/ramazankara/restore-drill`.
- Tightened configuration validation for provider/tool/repo/check compatibility.

## [0.1.0] - 2026-05-22

Initial public release.
