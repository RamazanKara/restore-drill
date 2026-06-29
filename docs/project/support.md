# Support, Upgrades, and Deprecation Policy

restore-drill follows semantic versioning and keeps v1 public contracts stable.
The goal is that scheduled restore drills keep running across patch and minor
updates unless a change is clearly documented here, in `CHANGELOG.md`, and in
the release notes.

## Supported versions

Security and critical restore correctness fixes are provided for the latest
released minor version.

For example, after `v1.2.0` is released, fixes target `v1.2.x`. Older minor
versions may receive fixes only when the maintainer chooses to backport a
specific low-risk patch.

## Stable public contracts

Within v1, these contracts should remain backward compatible:

- documented YAML fields
- CLI commands and flags
- provider names and documented backup tool names
- Prometheus metric names and label meanings
- run JSON field names, types, and meanings
- webhook payload envelope and embedded run result shape
- Helm values documented in [KUBERNETES.md](../guides/kubernetes.md)

Minor releases may add optional fields, flags, metrics, report fields, providers,
or Helm values.

## Deprecations

Deprecations should be documented in `CHANGELOG.md`, the relevant docs page, and
release notes.

Deprecations should include:

- what is deprecated
- why it is deprecated
- the replacement path
- the earliest release where removal may happen

Deprecated v1 behavior should remain available until the next major release
unless it is unsafe, broken, or impossible to support.

## Upgrade guidance

Before upgrading scheduled production drills:

1. Read `CHANGELOG.md` for the target release.
2. Run `restore-drill validate --config drill.yaml`.
3. Run one manual drill in a non-production environment with the same restore
   target image and backup repository class.
4. Confirm JSON/reporting consumers still accept the output.
5. Confirm Pushgateway alerts and webhook receivers still behave as expected.

For Helm upgrades, render the chart before applying:

```bash
helm template restore-drill deploy/helm/restore-drill \
  --namespace restore-drill \
  --set-file config.inline=drill.yaml
```

Then apply through your normal release process:

```bash
helm upgrade --install restore-drill deploy/helm/restore-drill \
  --namespace restore-drill \
  --set-file config.inline=drill.yaml
```
