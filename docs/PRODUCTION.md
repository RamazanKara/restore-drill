# Production Readiness

Use this checklist before relying on restore-drill for audit or disaster-recovery evidence.

## Deployment

- Run drills in a dedicated namespace.
- Use the Helm chart with `rbac.create=true`.
- Keep `concurrencyPolicy: Forbid` unless restore targets are known to be independent and resource headroom is reserved.
- Set `activeDeadlineSeconds` above the slowest expected restore and below the point where a stuck restore would become operational noise.
- Configure resource requests and limits for the restore-drill CronJob and for every `restore.container`.
- Use `runtime.serviceAccountName`, `runtime.imagePullSecrets`, `runtime.podLabels`, and `runtime.podAnnotations` when ephemeral restore pods need workload identity, private registry credentials, placement labels, or audit annotations.

## Secrets

- Do not store credentials in drill YAML.
- Inject credentials through Kubernetes Secrets, workload identity, IRSA, or environment interpolation.
- Scope backup credentials to read-only access where the backup tool supports it.
- Redact backup paths, bucket names, and restored data values before sharing logs.

## Restore target images

Each drill's `restore.container.image` must contain both the database runtime and the selected backup tool.

Examples:

- PostgreSQL pgBackRest: an image with `postgres`, `psql`, `pg_isready`, and `pgbackrest`
- PostgreSQL WAL-G: an image with `postgres`, `psql`, `pg_isready`, and `wal-g`
- MySQL xtrabackup: an image with `mysql` or `mariadb`, `mysqladmin` or `mariadb-admin`, a safe launcher (`mysqld_safe` or `mariadbd-safe`), and `xtrabackup`
- MariaDB mariabackup: an image with `mariadb`, `mariadb-admin`, `mariadbd-safe`, and `mariabackup`
- Redis: `redis:7-alpine` is enough for RDB/AOF restore drills

Preflight checks fail fast when required tools are missing.

For archive-based physical backups, include the archive tools too: `tar` for `.tar`, `.tar.gz`, and `.tgz`; `xbstream` for `.xbstream`; and `gzip` plus `xbstream` for `.xbstream.gz`.

## Observability

- Enable `metrics.prometheus.pushgateway`.
- Scrape Pushgateway with your existing Prometheus setup. With Prometheus Operator, use the Pushgateway chart or platform `ServiceMonitor`; restore-drill itself is a short-lived job and does not expose a stable scrape endpoint.
- Alert when `restore_drill_validation_passed == 0`.
- Alert when `time() - restore_drill_last_success_timestamp` exceeds the expected drill interval.
- Archive JSON or HTML reports from `restore-drill report` for audit evidence. HTML reports include per-check failure evidence for failed drills.
- Before relying on physical or PITR paths (`pgbackrest`, `wal-g`, `xtrabackup`, `mariabackup`) in production, run a drill against a real non-production backup repository and the exact restore image you plan to schedule. The providers support directory and archive staging for physical sources. The generated release fixture covers compressed logical PostgreSQL/MySQL dumps, real pgBackRest, real WAL-G, real Percona `xtrabackup`, real MariaDB `mariabackup`, and Redis AOF/RDB with restored key checks; the kind smoke test covers Kubernetes pod lifecycle with Redis AOF.

## Network policy

When enabling `networkPolicy.enabled`, allow egress to:

- DNS
- object storage or backup repositories
- Pushgateway
- webhook endpoints
- container registry endpoints if images are pulled at runtime

## Release gate

Before tagging a release, run:

```bash
make verify
RESTORE_DRILL_INTEGRATION=1 go test -race -count=1 -timeout=20m ./test/integration/...
make test-k8s
goreleaser release --snapshot --clean --skip=publish
```

The local release gate requires Go, Docker/Buildx, Helm, GoReleaser, Syft, kind, and kubectl. The kind smoke test uses `kindest/node:v1.31.4` by default because it is stable in WSL/Docker Desktop environments.
