# Maintainers

restore-drill is maintained by the repository owner:

- Ramazan Kara (`@RamazanKara`)

## Maintainer Responsibilities

- Keep the documented v1 YAML, CLI, JSON, metrics, and Helm contracts stable.
- Review restore correctness, security, and release-supply-chain changes with
  extra care.
- Keep vulnerability allowlist entries narrow, documented, and time-bound.
- Require tests or documented manual verification for provider, runtime,
  reporting, and release changes.

## Good First Issues

Good first contributions usually improve documentation, examples, test
coverage, diagnostics, or small provider edge cases without changing public
contracts.

Before opening a pull request:

```bash
make test-unit
make lint
make vuln
```

Use `make verify` when Helm and GoReleaser are installed locally.
