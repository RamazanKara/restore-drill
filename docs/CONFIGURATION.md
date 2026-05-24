# Configuration Reference

`restore-drill` reads YAML from `--config`. Environment interpolation happens
before YAML parsing and supports `${VAR}` plus `${VAR:-default}`.

## Top level

```yaml
drills: []
metrics:
  prometheus:
    enabled: false
    pushgateway: ""
    labels: {}
reporting:
  format: [table]
  output: ""
  retention: 90d
```

`drills` must contain at least one drill. Drill names must be unique.

## Drill

```yaml
drills:
  - name: production-postgres
    provider: postgres
    schedule: "manual"
    backup: {}
    restore: {}
    checks: []
    alerts: []
```

Supported providers are `postgres`, `mysql`, and `redis`.

`schedule` is informational in CLI configs. Scheduling is handled by CI, cron,
or the Helm CronJob.

## Backup

```yaml
backup:
  tool: pg_dump
  source: /backups/latest.sql.gz
```

`source` can be:

- a local host file or directory, copied into the restore target
- a path already mounted inside the restore target
- an `s3://bucket/key-or-prefix` URI

S3-compatible repository form:

```yaml
backup:
  tool: pg_dump
  repo:
    type: s3
    bucket: my-backups
    endpoint: https://s3.eu-central-1.amazonaws.com
    prefix: postgres/latest.sql.gz
    region: eu-central-1
```

When `prefix` ends with `/`, restore-drill downloads the newest object under
that prefix by modification time.

Physical restore sources for pgBackRest, WAL-G local repositories, xtrabackup,
and mariabackup can be directories or common archive files. restore-drill
expands `.tar`, `.tar.gz`, `.tgz`, `.xbstream`, and `.xbstream.gz` inside the
restore target before running the provider restore command.

## Provider tools

| Provider | `backup.tool` values | Required restore-image tools |
| --- | --- | --- |
| PostgreSQL | `pg_dump`, `pg_restore`, `pgbackrest`, `wal-g`, `walg` | `psql`, `pg_isready`, selected backup tool, `pg_ctl` for physical/PITR flows |
| MySQL/MariaDB | `mysqldump`, `xtrabackup`, `mariabackup` | `mysql` or `mariadb`, `mysqladmin` or `mariadb-admin`, selected backup tool |
| Redis | `rdb`, `aof` | `redis-server`, `redis-cli` |

Archive-based physical backups also need archive tools in the restore image:
`tar` for tar archives, `xbstream` for xbstream archives, and `gzip` for
compressed dumps or compressed archive streams.

Preflight checks run before restore and fail with actionable messages when a
required command is missing.

## Restore

```yaml
restore:
  target: latest
  timeout: 30m
  container:
    image: postgres:16
    env:
      POSTGRES_HOST_AUTH_METHOD: trust
    resources:
      memory: 2Gi
      cpu: "500m"
```

`target` is passed to PITR-capable providers. The `--target` CLI flag overrides
`restore.target` for every drill in the config.

`timeout` is a Go duration. If omitted, the drill timeout defaults to 10
minutes.

`container.image` is the disposable restore target image, not the restore-drill
CLI image. It must include the provider runtime and required tools.

CPU accepts whole cores such as `"1"` and Kubernetes-style millicores such as
`"500m"`. Memory accepts values such as `512Mi`, `2Gi`, `500M`, and `2G`.

## Checks

```yaml
checks:
  - name: users-exist
    type: query
    sql: "SELECT count(*) FROM users"
    expect: "> 0"
```

Supported check types:

| Provider | Check types |
| --- | --- |
| PostgreSQL | `query`, `sql`, `row_count`, `schema`, `freshness`, `extensions` |
| MySQL/MariaDB | `query`, `sql`, `row_count`, `schema`, `freshness` |
| Redis | `key_count`, `key_sample`, `query` |

For Redis `query`, the `sql` field is reused as a `redis-cli` command string,
for example `sql: "PING"`.

Supported expectations:

- numeric comparisons: `> 0`, `>= 10`, `< 900`, `== 1`, `!= 0`
- booleans: `true`, `false`, `exists`
- freshness: `age < 24h`, `age <= 30m`
- substring: `contains "text"`
- comma lists: `pgcrypto, uuid-ossp` requires all listed values in the actual
  comma-separated result

## Alerts

```yaml
alerts:
  - type: webhook
    url: https://hooks.example.invalid/restore-drill
    headers:
      Authorization: "Bearer ${RESTORE_DRILL_WEBHOOK_TOKEN}"
  - type: prometheus
    endpoint: http://pushgateway:9091
```

Webhook alerts send a JSON object with a summary and the same result shape as
run JSON output. `url` and `endpoint` are both accepted for webhooks; prefer
`url`.

Webhook `headers` are copied to the request and support environment
interpolation. Keep tokens in the runtime environment, not in committed config.

Prometheus Pushgateway is configured globally under `metrics.prometheus`.
Per-drill `prometheus` alert entries are accepted for documentation compatibility.

## Metrics

```yaml
metrics:
  prometheus:
    enabled: true
    pushgateway: http://pushgateway.monitoring:9091
    labels:
      environment: production
      team: platform
```

Metrics are pushed after each run when `enabled` is true and `pushgateway` is
set. Labels are used as Pushgateway grouping labels, except `environment`, which
is emitted as a metric label and defaults to `default`.

See [REPORTING.md](REPORTING.md) for metric names and alert examples.

## Reporting

```yaml
reporting:
  format: [json, html]
  output: ./reports/
  retention: 90d
```

When `output` is set, `restore-drill run` writes configured report files after
each run:

- `json`: current run result array, using the same shape as `--format json`
- `html`: compliance report generated from local history for the retention
  window
- `table`: accepted for stdout compatibility and does not create a file

`retention` defaults to `90d` and accepts day counts such as `30d` or Go
durations such as `720h`.

See [REPORTING.md](REPORTING.md) for output path rules and the JSON
compatibility policy.
