# Architecture

## Overview

restore-drill is a Go CLI and Kubernetes-native tool that orchestrates backup restoration into ephemeral environments, validates correctness, and publishes results as metrics and compliance reports.

## Design principles

1. **Ephemeral execution** — Restore targets are disposable containers. No permanent infrastructure required.
2. **Provider-based** — Each database/store type is a provider that implements a common interface.
3. **Validation as code** — Checks are declarative and composable.
4. **Observable by default** — Every drill emits Prometheus metrics, regardless of other output.
5. **Fail-loud** — A failed drill is always louder than a successful one.

## Component diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                        restore-drill CLI                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────┐   ┌──────────────┐   ┌────────────┐   ┌────────┐ │
│  │ Scheduler│──▶│ Drill Engine │──▶│ Validator  │──▶│Reporter│ │
│  └──────────┘   └──────┬───────┘   └────────────┘   └────────┘ │
│                         │                                        │
│              ┌──────────┴──────────┐                             │
│              │   Provider Layer    │                             │
│              ├─────────┬──────────┤                             │
│              │ postgres│ mysql    │                             │
│              │ redis   │ s3      │                             │
│              │ etcd    │ ...     │                             │
│              └─────────┴──────────┘                             │
│                         │                                        │
│              ┌──────────┴──────────┐                             │
│              │  Container Runtime  │                             │
│              │ (Docker / K8s Job)  │                             │
│              └─────────────────────┘                             │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Key components

### 1. Drill Engine (`pkg/engine/`)

The orchestrator. For each configured drill:

1. Resolves the backup to restore (latest, specific timestamp, or PITR target)
2. Provisions an ephemeral container via the runtime adapter
3. Delegates restore to the appropriate provider
4. Runs validation checks
5. Collects timing and result data
6. Tears down the container
7. Publishes metrics and reports

The engine supports:
- Sequential execution (default, resource-friendly)
- Parallel execution (for teams with multiple independent databases)
- Timeout enforcement per drill
- Retry with backoff on transient failures (network, container pull)

### 2. Provider interface (`pkg/providers/`)

```go
type Provider interface {
    // Name returns the provider identifier (e.g., "postgres", "mysql").
    Name() string

    // Restore pulls a backup and restores it into the target container.
    // Returns restore metadata (duration, backup timestamp, size).
    Restore(ctx context.Context, cfg BackupConfig, target Container) (*RestoreResult, error)

    // Validate runs provider-specific checks against the restored data.
    // Generic checks (query, key_count) are handled by the validator;
    // this is for provider-specific structural checks.
    Validate(ctx context.Context, target Container, checks []Check) (*ValidationResult, error)

    // Cleanup performs provider-specific teardown (drop temp roles, etc.).
    Cleanup(ctx context.Context, target Container) error
}
```

Each provider encapsulates:
- How to pull a backup from storage (S3, local, GCS)
- How to invoke the restore tool (pgBackRest, pg_restore, mysql, redis-server, etc.)
- How to connect to the restored instance for validation
- Provider-specific validation (e.g., PostgreSQL extension check, replication slot state)

### 3. Container runtime adapter

Two implementations:

| Adapter | Use case |
|---------|----------|
| Docker | Local development, CI pipelines |
| Kubernetes Job | Production CronJob deployments |

The adapter interface:

```go
type Runtime interface {
    // Create provisions a container/pod with the given image and resource limits.
    Create(ctx context.Context, spec ContainerSpec) (Container, error)

    // Exec runs a command inside the container and returns stdout/stderr.
    Exec(ctx context.Context, c Container, cmd []string) ([]byte, error)

    // CopyTo copies a file/stream into the container filesystem.
    CopyTo(ctx context.Context, c Container, dest string, src io.Reader) error

    // Destroy tears down the container and cleans up resources.
    Destroy(ctx context.Context, c Container) error

    // Logs returns the container's stdout/stderr logs.
    Logs(ctx context.Context, c Container) (io.ReadCloser, error)
}
```

### 4. Validator (`pkg/validator/`)

Runs checks against the restored data. Check types:

| Type | Description | Example |
|------|-------------|---------|
| `query` | Execute SQL/command, compare result | `SELECT count(*) FROM users` > 0 |
| `schema` | Verify schema version or structure | migration_version >= 142 |
| `extensions` | Check installed extensions/modules | pgcrypto, uuid-ossp |
| `key_count` | Count keys (Redis, S3) | > 1000 |
| `key_sample` | Verify specific keys exist | session:*, cache:* |
| `freshness` | Check data recency | max(updated_at) < 25h ago |
| `checksum` | Compare table/object checksums | optional integrity check |
| `custom` | Run arbitrary script/binary | user-defined |

Validation results include:
- Check name and type
- Expected vs. actual value
- Pass/fail status
- Execution duration
- Error message (on failure)

### 5. Reporter (`pkg/reporter/`)

Outputs drill results in multiple formats:

- **Prometheus** — Push metrics to Pushgateway (or expose via HTTP for scraping)
- **JSON** — Machine-readable result file
- **HTML** — Human-readable compliance report with charts
- **Webhook** — POST results to Slack, PagerDuty, or custom endpoint
- **stdout** — Colored table for CLI usage

### 6. Metrics (`pkg/metrics/`)

Wraps Prometheus client library. Key design decisions:

- Uses Pushgateway (not long-lived HTTP server) because drills are ephemeral
- Labels: `drill` (name), `provider`, `environment`, `status`
- Histograms for duration, gauges for last-success timestamp and backup age

## Data flow

```
                    Config (YAML)
                         │
                         ▼
              ┌─────────────────────┐
              │   Parse & Schedule  │
              └──────────┬──────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │  Provision Container│◀── Docker / K8s API
              └──────────┬──────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │  Pull Backup        │◀── S3 / local / GCS
              └──────────┬──────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │  Execute Restore    │◀── pgBackRest / pg_restore / mysql / ...
              └──────────┬──────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │  Wait for Ready     │   (health check loop)
              └──────────┬──────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │  Run Validations    │
              └──────────┬──────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │  Collect Results    │
              └──────────┬──────────┘
                         │
              ┌──────────┼──────────┐
              ▼          ▼          ▼
         Prometheus   Report     Webhook
         (metrics)    (HTML/JSON) (alert)
              │          │          │
              ▼          ▼          ▼
         Pushgateway  Filesystem  Slack/PD
```

## Security considerations

- **No production credentials** — The tool needs read access to backup storage, not to the production database.
- **Ephemeral containers** — Restored data exists only for the duration of the drill, then is destroyed.
- **Network isolation** — Kubernetes drills run in a dedicated namespace with restrictive NetworkPolicy (no egress except backup storage and metrics).
- **Secrets management** — Backup credentials are injected via Kubernetes Secrets or environment variables, never stored in config files.
- **Least privilege** — The ServiceAccount needs only: create/delete Jobs in its namespace, read Secrets, push metrics.

## Deployment modes

### Mode 1: CLI (development / CI)

```bash
restore-drill run --config drill.yaml
```

Uses Docker runtime. Pulls backup from configured storage, runs locally.

### Mode 2: Kubernetes CronJob (production)

Deployed via Helm chart. Creates a CronJob per drill. Each execution:
1. Starts a pod with restore-drill
2. restore-drill creates a child Job (the ephemeral database)
3. Runs validation against the child Job
4. Pushes metrics
5. Destroys child Job
6. Pod completes

### Mode 3: One-shot (incident response)

```bash
restore-drill run --config drill.yaml --target "2026-05-20T14:30:00Z" --no-cleanup
```

Restores to a specific PITR timestamp and keeps the container running for manual inspection. Useful during incident response when you need to check "what did the data look like at time X?"

## Extension points

- **Custom providers** — Implement the `Provider` interface for any backup/restore tool
- **Custom validators** — Add check types via the `Checker` interface
- **Custom reporters** — Implement the `Reporter` interface for any output target
- **Hooks** — Pre-restore and post-restore hooks for setup/teardown (e.g., create temp credentials)
