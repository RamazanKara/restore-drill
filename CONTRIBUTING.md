# Contributing to restore-drill

Thanks for helping improve restore-drill. This project is intended to be boring,
auditable infrastructure software: small changes, clear tests, and honest
documentation beat clever surprises.

## Development

Install the Go version required by `go.mod`, GNU Make, a POSIX shell, a C
compiler when CGO is enabled, and the golangci-lint/staticcheck versions pinned in
`.github/workflows/ci.yml`. Use WSL Ubuntu for Linux-only checks on Windows.

```bash
git clone https://github.com/RamazanKara/restore-drill.git
cd restore-drill
make build
make fmt-check vet
make test-unit
make test-fuzz
make lint staticcheck
make vuln
```

GitHub Actions is currently unavailable due to billing. Run these targets
locally before submitting changes; one CI workflow runs the same core checks.
`make cover` writes `coverage.out` and `coverage.html` locally. Tests skip `-race`
when `go env CGO_ENABLED` is `0`.

Use `make verify` before opening a pull request when Helm and GoReleaser are
installed locally. For provider or runtime changes, also run the relevant
integration gate when Docker or kind is available.

For first contributions, good issues are usually docs, examples, diagnostics,
test coverage, or small edge cases that do not change public v1 contracts.

## Pull requests

- Keep changes focused on one behavior or documentation area.
- Add or update tests for provider, runtime, config, metrics, or reporter
  changes.
- Update examples and docs when changing public YAML, CLI flags, metrics, or
  release behavior.
- Do not commit generated release artifacts, local binaries, credentials, or
  real backup data.
- Keep roadmap candidates clearly separate from GA behavior in README,
  [docs/project/roadmap.md](docs/project/roadmap.md), and release notes.

## Integration tests

Docker-backed integration tests are opt-in because they create containers:

```bash
make test-integration
```

Provider changes should include either unit coverage with a fake runtime or an
integration fixture that exercises the documented restore path.

## Documentation

Start with [docs/index.md](docs/index.md) to find the right page. README
should stay concise; detailed behavior belongs in the focused docs under
`docs/`.
