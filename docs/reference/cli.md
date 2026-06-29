# CLI reference

`restore-drill` is a single binary with a handful of subcommands. This page is
the complete command, flag, and exit-code reference. For the YAML those commands
consume, see the [configuration reference](../guides/configuration.md).

## Global flags

| Flag | Description |
| --- | --- |
| `-v`, `--verbose` | Enable debug logging (structured `log/slog` output on stderr). |
| `-h`, `--help` | Show help for any command. |

## `run`

Execute the drills in a config file: provision an ephemeral target, stage the
backup, restore it, run validation checks, then write reports and metrics.

```bash
restore-drill run --config drill.yaml --runtime docker
restore-drill run --config drill.yaml --runtime docker --parallel --format json
```

| Flag | Default | Description |
| --- | --- | --- |
| `-c`, `--config` | `drill.yaml` | Path to the drill configuration file. |
| `--runtime` | `auto` | Target runtime: `auto`, `docker`, or `kubernetes`. |
| `--format` | `table` | stdout format: `table` or `json`. |
| `--parallel` | `false` | Run drills concurrently instead of sequentially. |
| `--no-cleanup` | `false` | Keep the restore target after the drill for inspection (see [incident response](../guides/incident-response.md)). |
| `--target` | _none_ | PITR target timestamp (e.g. `2026-05-20T14:30:00Z`), applied to every drill. |
| `--kube-namespace` | `restore-drill` | Namespace for ephemeral restore pods (Kubernetes runtime). |
| `--kube-service-account` | _none_ | Service account for restore pods. |
| `--kube-pod-label` | _none_ | Label `key=value` for restore pods (repeatable). |
| `--kube-pod-annotation` | _none_ | Annotation `key=value` for restore pods (repeatable). |
| `--kube-image-pull-secret` | _none_ | Image pull secret name for restore pods (repeatable). |

`run` exits non-zero if any drill errors or fails validation, which makes it a
drop-in CI gate.

## `validate`

Parse and validate a config file without running anything. Catches schema and
compatibility errors (unknown provider/tool/check, missing required fields).

```bash
restore-drill validate --config drill.yaml
```

| Flag | Default | Description |
| --- | --- | --- |
| `-c`, `--config` | `drill.yaml` | Path to the drill configuration file. |

## `doctor`

Check that the local environment is ready: config validity, runtime
reachability, state/report path writability, and release tooling.

```bash
restore-drill doctor --config drill.yaml --runtime docker
restore-drill doctor --config drill.yaml --runtime kubernetes --format json --strict
```

| Flag | Default | Description |
| --- | --- | --- |
| `-c`, `--config` | `drill.yaml` | Path to the drill configuration file. |
| `--runtime` | `auto` | Runtime to check: `auto`, `docker`, or `kubernetes`. |
| `--format` | `table` | Output format: `table` or `json`. |
| `--strict` | `false` | Treat warnings as failures (exit non-zero on any warning). |

## `status`

Show the most recent run from local history as a table.

```bash
restore-drill status
```

No flags. Reads the latest run from the [state directory](../operations/state.md).

## `report`

Generate a restore-evidence report (HTML or JSON) aggregated from drill history.

```bash
restore-drill report --last 90 --output restore-evidence.html
restore-drill report --last 30 --format json
```

| Flag | Default | Description |
| --- | --- | --- |
| `--format` | `html` | Report format: `html` or `json`. |
| `--last` | `90` | Include drills from the last N days. |
| `-o`, `--output` | stdout | Output file path. |

See [reporting & alerts](reporting.md) for the report contents and the JSON
contract.

## `version`

Print the version, commit, and build date (set via ldflags at release time).

```bash
restore-drill version
```

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success — all drills passed (or the command completed normally). |
| `1` | Failure — a drill errored or failed validation, a config was invalid, or (for `doctor --strict`) a warning was raised. |

## See also

- [Configuration reference](../guides/configuration.md)
- [CI/CD integration](../guides/ci-cd.md)
- [Incident response](../guides/incident-response.md)
