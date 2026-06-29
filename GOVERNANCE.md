# Governance

restore-drill is a maintainer-led open source project. The project favors a
small, reliable restore-verification core over broad feature sprawl.

## Decision Rules

- v1 public contracts stay backward compatible unless a security issue makes
  that impossible.
- Provider and runtime changes need tests for restore behavior, failure
  reporting, and documented configuration.
- New providers should wait until the existing PostgreSQL, MySQL/MariaDB, and
  Redis paths remain boringly reliable.
- Security and release changes take priority over convenience features.

## Decision Authority

Final decisions rest with the maintainer ([MAINTAINERS.md](MAINTAINERS.md)).
Disagreements are resolved in the relevant issue or pull request, in the open;
the maintainer makes the final call when consensus is not reached. Proposals to
change this governance follow the same contribution path below.

## Contribution Path

1. Open an issue for behavior changes, new providers, or public contract changes.
2. Keep pull requests focused on one behavior or documentation area.
3. Update docs, examples, schemas, and changelog entries when public behavior changes.
4. Include local test output or explain which checks could not run.

## Vulnerability Handling

Security issues should follow [SECURITY.md](SECURITY.md). Public issues should
not include credentials, real backup data, sensitive repository names, or
retained restore target details.
