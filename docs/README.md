# Documentation

This directory is the operating manual for restore-drill. The README is the
front door; these pages go deeper into configuration, deployment, evidence, and
release work.

## Start here

- [Configuration reference](CONFIGURATION.md): YAML fields, provider tools,
  checks, alerts, metrics, and reporting.
- [Kubernetes and Helm](KUBERNETES.md): CronJob install, RBAC, runtime pod
  settings, secrets, network policy, and metrics guidance.
- [Reporting and alerts](REPORTING.md): JSON compatibility, HTML reports,
  webhooks, Pushgateway metrics, and Prometheus alert examples.
- [Production readiness](PRODUCTION.md): rollout checklist for scheduled restore
  drills in real environments.

## Integration and operations

- [CI/CD integration](ci-integration.md): GitHub Actions, GitLab CI, ArgoCD
  hooks, scheduled drills, incident mode, and exit codes.
- [Architecture](ARCHITECTURE.md): engine, runtime, provider, staging, state,
  reporting, and security model.
- [Release process](RELEASE.md): local release gates, snapshot validation, tags,
  artifacts, and release hygiene.
- [Support and upgrades](SUPPORT.md): supported versions, stable contracts,
  deprecations, and upgrade checks.
- [Release readiness and roadmap](IMPLEMENTATION_PLAN.md): current v1 scope,
  non-goals, hardening backlog, and future candidates.

## Project documents

- [Contributing](../CONTRIBUTING.md)
- [Security policy](../SECURITY.md)
- [Code of conduct](../CODE_OF_CONDUCT.md)
- [Changelog](../CHANGELOG.md)
