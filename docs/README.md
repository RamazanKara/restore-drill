# Documentation

This directory is the operating manual for restore-drill. The README is the
front door; these pages go deeper into configuration, deployment, evidence, and
release work.

## Start Here

New to the project? Start with the self-contained Redis demo in the root
[README](../README.md#quick-start). It builds the CLI, restores a tiny AOF
backup into Docker, and shows the pass/fail evidence table.

After that, choose the page that matches what you are doing:

| Goal | Read |
| --- | --- |
| Write or review a drill YAML file | [Configuration reference](CONFIGURATION.md) |
| Schedule restore drills in Kubernetes | [Kubernetes and Helm](KUBERNETES.md) |
| Keep reports, webhooks, and Prometheus alerts useful | [Reporting and alerts](REPORTING.md) |
| Roll out scheduled drills safely | [Production readiness](PRODUCTION.md) |
| Wire restore drills into CI/CD | [CI/CD integration](ci-integration.md) |
| Understand the internal model before changing code | [Architecture](ARCHITECTURE.md) |

## Reference Pages

- [Release process](RELEASE.md): local release gates, snapshot validation, tags,
  artifacts, and release hygiene.
- [Support and upgrades](SUPPORT.md): supported versions, stable contracts,
  deprecations, and upgrade checks.
- [Roadmap and release readiness](ROADMAP.md): current v1 scope, non-goals,
  maintenance priorities, and future candidates.

## Project Documents

- [Contributing](../CONTRIBUTING.md)
- [Security policy](../SECURITY.md)
- [Code of conduct](../CODE_OF_CONDUCT.md)
- [Changelog](../CHANGELOG.md)
