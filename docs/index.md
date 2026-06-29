# restore-drill

**Automated backup restore verification for self-hosted infrastructure.**

Backups that are never restored are guesses. `restore-drill` restores real backup
artifacts into disposable Docker containers or Kubernetes pods, validates the
restored data, records RTO/RPO evidence, and publishes machine-readable results
for audits and alerts.

It is intentionally focused on one job: proving that backups can be restored. It
does **not** schedule backups, manage repositories, estimate costs, inventory
infrastructure, or replace observability platforms.

## Start here

<div class="grid cards" markdown>

- :material-download: **[Installation](getting-started/installation.md)** — install the CLI and container image, and verify signatures.
- :material-rocket-launch: **[Quick start](getting-started/quickstart.md)** — run the self-contained Redis demo in two commands.
- :material-file-cog: **[Configuration](guides/configuration.md)** — the full drill YAML reference.
- :material-console: **[CLI reference](reference/cli.md)** — every command, flag, and exit code.

</div>

## Find what you need

| Your goal | Read |
| --- | --- |
| Install and run a first drill | [Installation](getting-started/installation.md) → [Quick start](getting-started/quickstart.md) |
| Write or review a drill config | [Configuration](guides/configuration.md) |
| Look up a command or flag | [CLI reference](reference/cli.md) |
| Deploy on Kubernetes with Helm | [Kubernetes guide](guides/kubernetes.md) |
| Schedule drills in CI/CD | [CI/CD integration](guides/ci-cd.md) |
| Recover from a real incident | [Incident response](guides/incident-response.md) |
| Wire up reports, webhooks, Slack, metrics | [Reporting & alerts](reference/reporting.md) |
| Roll out to production safely | [Production rollout](operations/production.md) |
| Understand history & evidence retention | [State & history](operations/state.md) |
| Diagnose a failing drill | [Troubleshooting](reference/troubleshooting.md) |
| Understand the internals | [Architecture](reference/architecture.md) |
| Integrate the machine-readable contracts | [Schemas](reference/schemas/index.md) |
| Know what is stable and supported | [Support policy](project/support.md) · [Roadmap](project/roadmap.md) |

## Project

Source, issues, and releases live on
[GitHub](https://github.com/RamazanKara/restore-drill). Community and governance
documents:
[Contributing](https://github.com/RamazanKara/restore-drill/blob/main/CONTRIBUTING.md)
·
[Security](https://github.com/RamazanKara/restore-drill/blob/main/SECURITY.md)
·
[Governance](https://github.com/RamazanKara/restore-drill/blob/main/GOVERNANCE.md)
·
[Maintainers](https://github.com/RamazanKara/restore-drill/blob/main/MAINTAINERS.md)
·
[Changelog](https://github.com/RamazanKara/restore-drill/blob/main/CHANGELOG.md).
