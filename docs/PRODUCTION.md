# Production Readiness

Use this checklist before relying on restore-drill for audit evidence,
disaster-recovery readiness, or scheduled operational alerts.

## Rollout Path

1. Run `restore-drill doctor --config drill.yaml --runtime docker` or
   `--runtime kubernetes` to check config, runtime access, state/report paths,
   and local release tooling.
2. Run the local Redis demo to confirm the CLI and runtime work on your machine.
3. Create one drill against a non-production backup and the exact restore image
   you expect to schedule.
4. Add JSON/HTML reporting and verify the artifacts are stored durably.
5. Add Pushgateway metrics and alert on failed validation plus stale success.
6. Move the same config into CI or Helm, then keep the first few runs under
   manual review.

## Deployment

- Run restore-drill in a dedicated namespace or isolated CI runner.
- Use the Helm chart with `rbac.create=true` for Kubernetes schedules.
- Keep `concurrencyPolicy: Forbid` unless overlapping restores are intentional.
- Set `activeDeadlineSeconds` above the slowest expected restore and below the
  point where a stuck restore becomes operational noise.
- Configure resource requests and limits for the CronJob and for every
  `restore.container`.
- Use restore target pod labels, annotations, service accounts, and image pull
  secrets when workload identity, private registries, placement, or audit tags
  are required.

## Restore target images

Each drill's `restore.container.image` must contain the database runtime,
client, and selected backup tool.

Common examples:

- PostgreSQL pgBackRest: `postgres`, `psql`, `pg_isready`, `pg_ctl`,
  `pgbackrest`
- PostgreSQL WAL-G: `postgres`, `psql`, `pg_isready`, `pg_ctl`, `wal-g` or
  `walg`
- MySQL xtrabackup: `mysql` or `mariadb`, `mysqladmin` or `mariadb-admin`,
  `mysqld_safe` or `mariadbd-safe`, `xtrabackup`
- MariaDB mariabackup: `mariadb`, `mariadb-admin`, `mariadbd-safe`,
  `mariabackup`
- Redis: `redis-server`, `redis-cli`

Archive-based physical backups may also require `tar`, `gzip`, or `xbstream`.
Local/S3 staging also requires `tar` in the restore target image.
Preflight checks fail fast when required commands are missing.

## Secrets

- Do not store credentials in drill YAML.
- Inject credentials through environment interpolation, Kubernetes Secrets,
  workload identity, IRSA, or cloud-native credential chains.
- Scope backup credentials to read-only access where the backup tool supports
  it.
- Use webhook `headers` for authentication tokens, with values sourced from the
  runtime environment.
- Redact backup URLs, bucket names, command output, and restored data values
  before sharing logs.

## Evidence and retention

- Keep `~/.restore-drill/history` or configured report output on durable
  storage when audit evidence must survive pod or runner cleanup.
- Use `reporting.format: [json, html]` with a durable `reporting.output` path
  when every scheduled run should leave file artifacts.
- Archive HTML reports for human review and JSON reports for automation.
- HTML evidence reports include per-check failure evidence, expected values,
  actual values, provider errors, and RTO/RPO summaries.
- Treat retained `--no-cleanup` targets as sensitive because they may contain
  restored production-like data.

## Observability

- Enable `metrics.prometheus.pushgateway` for scheduled runs.
- Scrape Pushgateway with your existing Prometheus setup.
- With Prometheus Operator, use a `ServiceMonitor` for the Pushgateway service,
  not for restore-drill CronJob pods.
- Alert when `restore_drill_validation_passed == 0`.
- Alert when `time() - restore_drill_last_success_timestamp` exceeds the
  expected drill interval.
- Alert when `restore_drill_backup_age_seconds` exceeds the allowed RPO window.

Detailed report file behavior, webhook payloads, metrics, and alert examples are
in [REPORTING.md](REPORTING.md).

## Network policy

When enabling `networkPolicy.enabled`, allow egress to:

- DNS
- object storage or backup repositories
- Pushgateway
- webhook endpoints
- container registry endpoints if images are pulled at runtime

## First production drill

Before relying on physical or PITR paths in production, run a drill against a
real non-production backup repository and the exact restore image you plan to
schedule. This applies especially to `pgbackrest`, `wal-g`/`walg`,
`xtrabackup`, and `mariabackup`.

## Release gate

Before tagging a release, run:

```bash
make verify
make test-integration
make test-k8s
goreleaser release --snapshot --clean --skip=publish
```

The local release gate requires Go, Docker/Buildx, Helm, GoReleaser, Syft, kind,
kubectl, Cosign, and govulncheck. `make vuln` allows only reviewed
no-fixed-version Docker/Moby advisories listed in `.govulncheck.allowlist`.
