# Contributing to restore-drill

Thanks for helping improve restore-drill. This project is intended to be boring,
auditable infrastructure software: small changes, clear tests, and honest
documentation beat clever surprises.

## Development

```bash
git clone https://github.com/RamazanKara/restore-drill.git
cd restore-drill
make build
make test-unit
make lint
```

Use `make verify` before opening a pull request when Helm and GoReleaser are
installed locally. For provider or runtime changes, also run the relevant
integration gate when Docker or kind is available.

## Pull requests

- Keep changes focused on one behavior or documentation area.
- Add or update tests for provider, runtime, config, metrics, or reporter
  changes.
- Update examples and docs when changing public YAML, CLI flags, metrics, or
  release behavior.
- Do not commit generated release artifacts, local binaries, credentials, or
  real backup data.
- Keep roadmap candidates clearly separate from GA behavior in README,
  [docs/ROADMAP.md](docs/ROADMAP.md), and release notes.

## Integration tests

Docker-backed integration tests are opt-in because they create containers:

```bash
make test-integration
```

Provider changes should include either unit coverage with a fake runtime or an
integration fixture that exercises the documented restore path.

## Documentation

Start with [docs/README.md](docs/README.md) to find the right page. README
should stay concise; detailed behavior belongs in the focused docs under
`docs/`.
