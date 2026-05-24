# Architecture

restore-drill is a Go CLI that orchestrates backup restores into ephemeral Docker containers or Kubernetes pods, validates the restored data, and emits operational and compliance evidence.

It is intentionally narrow. restore-drill verifies restores; it does not schedule backups, manage backup repositories, inventory infrastructure, estimate cloud cost, or replace monitoring systems.

## Components

```text
Config YAML
  -> CLI command
  -> Drill engine
  -> Runtime adapter: Docker or Kubernetes
  -> Provider: PostgreSQL, MySQL/MariaDB, or Redis
  -> Reporter: stdout, JSON, webhook, local history, Prometheus Pushgateway
```

## Drill engine

For each drill, the engine:

1. Applies the per-drill timeout.
2. Builds a container spec from `restore.container`.
3. Creates the target through the selected runtime.
4. Runs provider preflight checks for required commands.
5. Delegates restore and validation to the provider.
6. Records timing, backup age, validation results, and retained target details.
7. Runs provider cleanup and destroys the target unless `--no-cleanup` is set.
8. Publishes the configured reporter output.

Sequential execution is the default. `--parallel` runs independent drills concurrently while preserving result slots for reporting.

## Provider interface

```go
type Provider interface {
    Name() string
    Restore(ctx context.Context, rt Runtime, cfg BackupConfig, target Container) (*RestoreResult, error)
    Validate(ctx context.Context, rt Runtime, target Container, checks []Check) (*ValidationResult, error)
    Cleanup(ctx context.Context, target Container) error
}

type PreflightProvider interface {
    Preflight(ctx context.Context, rt Runtime, cfg BackupConfig, target Container, checks []Check) error
}
```

Providers are responsible for tool-specific restore and validation behavior. Shared backup staging is provided by `pkg/backup` for local files, mounted target paths, and S3-compatible objects.

## Runtime interface

```go
type Runtime interface {
    Create(ctx context.Context, spec ContainerSpec) (Container, error)
    Exec(ctx context.Context, c Container, cmd []string) ([]byte, error)
    CopyTo(ctx context.Context, c Container, dest string, src io.Reader) error
    Destroy(ctx context.Context, c Container) error
    Logs(ctx context.Context, c Container) (io.ReadCloser, error)
}
```

Docker is used for local and CI runs. Kubernetes creates ephemeral pods in the configured namespace and uses pod exec/copy/log APIs.

## Backup staging

`pkg/backup.Stage` resolves backup input into a path inside the restore target:

- existing local host files/directories are tar-copied into `/tmp/restore-drill-backups`
- missing local paths are treated as already mounted inside the target
- `s3://bucket/key` and `repo.type: s3` downloads are copied into the target
- a prefix ending in `/` selects the latest object by modification time

Credentials are sourced from the AWS SDK default chain or the target container environment, depending on the selected provider path.

## Validation

Validation checks are declarative. Providers execute SQL, Redis commands, or provider-specific checks and return actual values. The engine evaluates expectations such as numeric comparisons, booleans, `exists`, freshness expressions, substring matches, and comma-list containment.

## State and reports

Each run writes:

- latest run state: `~/.restore-drill/last-run.json`
- history entry: `~/.restore-drill/history/<timestamp>.json`

State writes are atomic. The latest-run file is replaced through a synced temporary file and rename. History entries are finalized without replacing existing files, so concurrent runs with the same timestamp keep separate entries.

The `report` command builds HTML or JSON compliance reports from local history.

## Security model

- restore-drill needs read access to backup storage, not production database access
- credentials should come from environment variables, Kubernetes Secrets, or workload identity
- restored data is destroyed after each run unless `--no-cleanup` is set
- Kubernetes RBAC is namespace-scoped to pod lifecycle, pod exec, and pod logs
- NetworkPolicy should allow only DNS, backup storage, metrics, and alert destinations
