# Configuration Reference

`restore-drill` reads YAML from `--config`. Environment interpolation supports `${VAR}` and `${VAR:-default}` before YAML parsing.

## Top level

```yaml
drills: []
metrics:
  prometheus:
    enabled: false
    pushgateway: ""
    labels: {}
reporting:
  format: [json, html]
  output: ./reports
  retention: 90d
```

`drills` must contain at least one drill. Drill names must be unique.

## Drill

```yaml
- name: production-postgres
  provider: postgres
  schedule: "manual"
  backup: {}
  restore: {}
  checks: []
  alerts: []
```

Supported providers are `postgres`, `mysql`, and `redis`.

## Backup

```yaml
backup:
  tool: pg_dump
  source: /backups/latest.sql.gz
```

`source` can be:

- a local host file or directory, copied into the restore target
- a path already mounted inside the target
- an `s3://bucket/key-or-prefix` URI

S3-compatible repo form:

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

When `prefix` ends with `/`, restore-drill downloads the newest object under that prefix.

Provider-specific tools:

- PostgreSQL: `pg_dump`, `pg_restore`, `pgbackrest`, `wal-g`, `walg`
- MySQL/MariaDB: `mysqldump`, `xtrabackup`, `mariabackup`
- Redis: `rdb`, `aof`

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

`target` is used by PITR-capable tools. CPU accepts whole cores such as `"1"` and Kubernetes-style millicores such as `"500m"`.

The restore image must include the selected provider client and backup tool. Preflight checks fail before restore if commands are missing.

## Checks

```yaml
checks:
  - name: users-exist
    type: query
    sql: "SELECT count(*) FROM users"
    expect: "> 0"
```

Supported check types:

- PostgreSQL/MySQL: `query`, `sql`, `row_count`, `schema`, `freshness`
- PostgreSQL only: `extensions`
- Redis: `key_count`, `key_sample`, `query`

Supported expectations:

- numeric comparisons: `> 0`, `>= 10`, `< 900`, `== 1`, `!= 0`
- booleans: `true`, `false`, `exists`
- freshness: `age < 24h`, `age <= 30m`
- substring: `contains "text"`
- comma lists: `pgcrypto, uuid-ossp` requires all listed values in the actual comma-separated result

## Alerts

```yaml
alerts:
  - type: webhook
    url: https://hooks.example.invalid/restore-drill
  - type: prometheus
    endpoint: http://pushgateway:9091
```

Webhook alerts send the same result shape as JSON output. Prometheus Pushgateway is configured globally under `metrics.prometheus`; per-drill prometheus alert entries are accepted for documentation compatibility.
