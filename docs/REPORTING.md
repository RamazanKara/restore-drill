# Reporting and Alerts

restore-drill writes operational evidence in four places:

- stdout from `restore-drill run`
- local state and history under `~/.restore-drill`
- optional configured JSON/HTML report files
- optional webhooks and Prometheus Pushgateway metrics

Use the outputs together rather than treating one format as the source of
truth:

| Need | Best output |
| --- | --- |
| Human operator watching a manual drill | stdout table |
| CI gate or audit pipeline | run JSON |
| Periodic review by humans | HTML compliance report |
| Paging or chat notification | webhook |
| SLO/RPO alerting | Pushgateway metrics |

## Configured report files

The `reporting` block controls files written after `restore-drill run` completes:

```yaml
reporting:
  format: [json, html]
  output: ./reports/
  retention: 90d
```

`output` is treated as a directory when it already exists as a directory, ends
with `/`, has no file extension, or more than one file format is enabled.
Generated file names include the run timestamp:

- `restore-drill-run-20260524T120000Z.json`
- `restore-drill-compliance-20260524T120000Z.html`

When a single format is enabled and `output` has a file extension, restore-drill writes that exact path.

`format` accepts:

- `json`: the per-run drill result array, using the same shape as
  `restore-drill run --format json`
- `html`: a compliance report generated from local history for the configured
  retention window
- `table`: accepted for stdout compatibility; it does not create a file

`retention` defaults to `90d`. It accepts day counts like `30d` and Go
durations like `720h`.

## JSON compatibility

The run JSON report is a top-level array of drill result objects. The v1 schema
is intentionally stable for automation and audit pipelines.

Fields currently emitted per drill:

| Field | Meaning |
| --- | --- |
| `name` | Drill name from config. |
| `provider` | Provider name, for example `postgres`, `mysql`, or `redis`. |
| `status` | `pass` or `fail`. |
| `started_at` | RFC3339 start timestamp. |
| `duration` | Human-readable Go duration. |
| `duration_ms` | Duration in milliseconds. |
| `backup_timestamp` | RFC3339 timestamp reported by the provider, when known. |
| `backup_age` | Backup age rounded to seconds, when known. |
| `validation_passed` | Whether all validation checks passed. |
| `error` | Drill-level error text, when present. |
| `cleanup_skipped` | Present when the target was retained with `--no-cleanup`. |
| `target_id` | Retained or executed target container/pod ID, when available. |
| `target_host` | Target host, when available. |
| `target_ports` | Container or pod port mapping, when available. |
| `checks` | Per-check validation evidence. |

Each check contains `name`, `type`, `expected`, `actual`, `passed`, `duration`, and optional `error`.

Compatibility policy for v1:

- Existing fields keep their names, JSON types, and meanings for all v1 releases.
- New fields may be added in minor releases.
- Fields may become omitted only when they are already optional and empty.
- Breaking schema changes wait for a new major version.

## HTML compliance reports

HTML reports are generated from local history and include:

- total, passed, and failed drill counts
- success rate
- average and maximum RTO
- compliance control status
- drill history
- per-check failure evidence with expected, actual, and error text

Use the explicit report command when you want an ad hoc report for a different window:

```bash
restore-drill report --last 90 --output compliance-report.html
restore-drill report --format json --last 30
```

## Webhooks

Webhook alerts are configured per drill. Header values support the same
environment interpolation as the rest of the config, so secrets can come from
the process environment instead of being committed to YAML:

```yaml
alerts:
  - type: webhook
    url: ${RESTORE_DRILL_WEBHOOK_URL}
    headers:
      Authorization: "Bearer ${RESTORE_DRILL_WEBHOOK_TOKEN}"
```

The webhook body is a JSON object with `timestamp`, `summary`, and `results`.
`results` uses the same drill result shape as the run JSON report.
restore-drill retries transport errors and HTTP 5xx responses, but it does not
retry HTTP 4xx responses.

## Prometheus

restore-drill is usually a short-lived job or CronJob, so it pushes metrics to
Prometheus Pushgateway instead of exposing a long-lived `/metrics` endpoint:

```yaml
metrics:
  prometheus:
    enabled: true
    pushgateway: http://pushgateway.monitoring:9091
    labels:
      environment: production
      team: platform
```

Scrape the Pushgateway with your existing Prometheus setup. With Prometheus
Operator, attach a `ServiceMonitor` to the Pushgateway service, not to
restore-drill CronJob pods.

Example alert expressions:

```promql
restore_drill_validation_passed == 0
```

```promql
time() - restore_drill_last_success_timestamp > 93600
```

```promql
restore_drill_backup_age_seconds > 86400
```

The Pushgateway write replaces the current restore-drill grouping on every run,
so repeated CronJob executions publish current-run values instead of
accumulating stale counters.
