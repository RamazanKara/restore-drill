# Architecture

restore-drill is a Go CLI that restores backup artifacts into ephemeral Docker
containers or Kubernetes pods, validates the restored data, and emits
operational evidence.

It is intentionally narrow. restore-drill verifies restores; it does not
schedule backups, manage backup repositories, inventory infrastructure, estimate
costs, or replace monitoring systems.

## Flow

```text
Config YAML
  -> CLI command
  -> Drill engine
  -> Runtime adapter: Docker or Kubernetes
  -> Provider: PostgreSQL, MySQL/MariaDB, Redis
  -> Reporter: stdout, JSON, webhook, local history, Prometheus Pushgateway
```

For each drill, the engine:

1. Applies the per-drill timeout.
2. Builds a restore target spec from `restore.container`.
3. Creates the target through the selected runtime.
4. Runs provider preflight checks for required commands.
5. Stages backup input when needed.
6. Delegates restore and validation to the provider.
7. Evaluates check expectations and preserves provider check errors.
8. Records timing, backup age, validation results, and retained target details.
9. Runs provider cleanup and destroys the target unless `--no-cleanup` is set.
10. Publishes the configured reporter output.

Sequential execution is the default. `--parallel` runs independent drills
concurrently while preserving result slots for reporting.

The important boundary is that the engine owns orchestration and evidence, while
providers own database-specific restore and validation behavior. Runtime adapters
only know how to create, execute in, copy to, log, and destroy disposable
targets.

## Engine interfaces

Providers implement tool-specific restore and validation:

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

Runtimes abstract Docker containers and Kubernetes pods:

```go
type Runtime interface {
    Create(ctx context.Context, spec ContainerSpec) (Container, error)
    Exec(ctx context.Context, c Container, cmd []string) ([]byte, error)
    CopyTo(ctx context.Context, c Container, dest string, src io.Reader) error
    Destroy(ctx context.Context, c Container) error
}
```

## Runtimes

Docker is used for local and CI runs. The Docker runtime creates one container
per drill, copies staged input into it, executes provider commands, and removes
the container after the drill unless retained.

Kubernetes creates one restore target pod per drill and uses pod exec, copy,
and delete APIs. Helm schedules the CLI as a CronJob and passes
`--runtime=kubernetes`.

## Backup staging

`pkg/backup.Stage` resolves backup input into a path inside the restore target:

- existing local host files and directories are tar-copied into
  `/tmp/restore-drill-backups`
- missing local paths are treated as already mounted inside the target
- `s3://bucket/key` and `repo.type: s3` inputs are downloaded locally and copied
  into the target
- a prefix ending in `/` selects the latest object by modification time

Physical restore archives are materialized inside the target before provider
restore commands run.

## Validation

Providers execute SQL, Redis commands, or provider-specific checks and return
actual values plus provider-level errors. The engine then evaluates expectations:

- numeric comparisons
- booleans and `exists`
- freshness expressions
- substring matches
- comma-list containment

Provider errors are preserved in JSON, HTML, webhooks, and state history.

## State and reports

Each run writes:

- latest run state: `~/.restore-drill/last-run.json`
- history entry: `~/.restore-drill/history/<timestamp>.json`

State writes are atomic. The latest-run file is replaced through a synced
temporary file and rename. History entries are finalized without replacing
existing files, so concurrent runs with the same timestamp keep separate
entries.

The `report` command builds HTML or JSON evidence reports from local history.
The `reporting` config block can also write per-run JSON and evidence HTML
artifacts automatically after `restore-drill run`.

## Security model

- restore-drill needs read access to backup storage, not production database
  access
- credentials should come from environment variables, Kubernetes Secrets, or
  workload identity
- restored data is destroyed after each run unless `--no-cleanup` is set
- retained targets should be treated as sensitive restored data
- Kubernetes RBAC is namespace-scoped to pod lifecycle, pod exec, and pod logs
- NetworkPolicy should allow only DNS, backup storage, metrics, and alert
  destinations
