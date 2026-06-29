# Troubleshooting

Start with `restore-drill doctor --config drill.yaml --runtime <docker|kubernetes>`;
it checks config validity, runtime reachability, and path writability before you
run a real drill. The failures below are the common ones.

## Preflight: required command not found in restore image

```
preflight: required command "psql" not found in restore image
```

The restore target image is missing a tool the provider needs. Each provider
requires its database client/server plus the selected backup tool, and local/S3
staging also requires `tar`. Use an image that bundles them (see the
[restore target images](../guides/configuration.md#provider-tools)
section) and re-run.

## Docker: cannot connect to the daemon

```
create container: ... Cannot connect to the Docker daemon
```

The Docker runtime can't reach a daemon. Confirm Docker is running and your user
can talk to it (`docker info`). In CI, ensure a Docker service / `setup-buildx`
step is present.

## Kubernetes: pods never become ready

- `restore-drill doctor --runtime kubernetes` failing means the API isn't
  reachable or the namespace/RBAC is missing. The Helm chart creates the
  namespace-scoped RBAC the restore pods need.
- A pod stuck `Pending` is usually an unschedulable image or missing pull secret
  — pass `--kube-image-pull-secret` (or set it in the chart values).
- A local/kind image must be loaded into the cluster and use a non-`:latest` tag
  so the default `IfNotPresent` pull policy uses it.

## Validation check fails unexpectedly

Inspect the evidence: every check records its `expected` and `actual` values in
the stdout table, run JSON, and HTML report. Common causes:

- The expectation expression doesn't match the actual type (e.g. a numeric
  comparison against non-numeric output).
- For `freshness`, the SQL must return a parseable timestamp.
- For etcd `key_count`, an empty `key` counts the whole keyspace; set a prefix to
  scope it.

Re-run with `--no-cleanup` and inspect the restored target directly (see
[incident response](../guides/incident-response.md)).

## Backup staging fails

```
create target staging directory: ... not found
```

Staging streams the backup into the target as a tar archive, so the restore image
must contain `tar`. For S3 sources, confirm credentials are present in the
environment and the bucket/prefix is correct.

## Alerts not delivered

- A `webhook` alert sends restore-drill's own JSON shape; chat platforms reject
  it. Use the `slack` alert type for Slack/Mattermost.
- With `on: failure`, alerts only fire when a drill fails — that's expected on a
  passing run.

## See also

- [CLI reference](cli.md) — `doctor`, `--no-cleanup`.
- [Configuration reference](../guides/configuration.md)
- [Reporting & alerts](reporting.md)
