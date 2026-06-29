# State & history

restore-drill records every run locally so you can review the latest result
(`status`) and generate evidence over time (`report`). This page explains where
that data lives, how it's retained, and how to keep it for audits.

## Where state is stored

| Data | Path |
| --- | --- |
| Latest run | `~/.restore-drill/last-run.json` |
| Run history | `~/.restore-drill/history/` (one timestamped entry per run) |

If no home directory is available (e.g. some CI sandboxes), restore-drill falls
back to `/tmp/.restore-drill/`.

Writes are **atomic**: the latest-run file is replaced atomically, and each
history entry is written under a unique timestamped name so concurrent runs never
clobber each other.

## Reading state

```bash
restore-drill status                          # latest run, as a table
restore-drill report --last 90 --format json  # aggregate the last 90 days
restore-drill report --last 90 --output evidence.html
```

`status` reads `last-run.json`; `report` aggregates the history directory within
the `--last N` day window. See [reporting & alerts](../reference/reporting.md) for
the report contents and JSON contract.

## Retention

The `reporting.retention` config field (e.g. `90d`) governs how long configured
report files and history are kept. Choose a window that satisfies your audit
requirements.

## Persisting evidence for audits

The local state directory is convenient but ephemeral — on CI runners or
disposable hosts it disappears with the machine. For durable evidence:

- Set `reporting.output` to a durable path (mounted volume, object storage sync)
  and enable `reporting.format: [json, html]` so each run writes self-contained
  report files.
- In CI, upload the generated reports as build artifacts or push them to object
  storage (see [CI/CD integration](../guides/ci-cd.md)).
- Back up `~/.restore-drill/history/` if you rely on `report` aggregation across
  runs on a long-lived host.

## See also

- [Reporting & alerts](../reference/reporting.md)
- [Production rollout](production.md)
- [Architecture](../reference/architecture.md) — how state is written.
