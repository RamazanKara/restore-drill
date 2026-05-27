# Release Process

restore-drill releases are tag driven. The canonical module is
`github.com/RamazanKara/restore-drill`, and container images are published to
`ghcr.io/ramazankara/restore-drill`.

This page is for maintainers cutting a release. Users looking for installation
options should start with the root [README](../README.md#install).

## Release toolchain

Install these tools before running the full local release gate:

- Go
- Docker with Buildx
- Helm
- GoReleaser
- Syft, used by GoReleaser for SBOM generation
- kind and kubectl for Kubernetes smoke tests

## Local release gate

```bash
make verify
make docker-smoke
make test-k8s
make test-integration
goreleaser release --snapshot --clean --skip=publish
cd dist && sha256sum -c checksums.txt
```

`make verify` runs build, vet, race-enabled unit tests, lint, example
validation, Helm lint/template, and `goreleaser check`.

`make test-integration` runs real Docker restore fixtures for the supported
provider matrix. `make test-k8s` runs the kind-backed Kubernetes smoke test.

Do not commit `dist/`, local binaries, credentials, or real backup data.

## GoReleaser note

GoReleaser currently warns that its stable `dockers` and `docker_manifests`
keys will eventually be replaced by `dockers_v2`. `dockers_v2` is still marked
experimental in GoReleaser 2.15.4, so release builds intentionally keep the
stable Docker path until the replacement is production-ready.

## Tagging

```bash
git tag -a vX.Y.Z -m "restore-drill vX.Y.Z"
git push origin vX.Y.Z
```

The release workflow builds:

- Linux and Darwin binaries for amd64 and arm64
- archive checksums
- SBOMs
- multi-architecture GHCR images
- `latest` and versioned container tags

## Release hygiene

Before publishing a tag:

- README, examples, Helm chart, and release links use the
  `RamazanKara/restore-drill` namespace.
- GHCR references use lowercase `ghcr.io/ramazankara/restore-drill`.
- Roadmap items are clearly marked as roadmap, not implied GA behavior.
- `CHANGELOG.md` has an entry for the release.
- Provider claims in README and docs are backed by tests or marked as roadmap.
- Release assets include checksums and SBOMs.
- No credentials, real backup data, generated binaries, or release artifacts are
  committed.

## Versioning

restore-drill follows semantic versioning:

- Patch releases fix bugs without changing public YAML, CLI, metrics, or JSON
  contracts.
- Minor releases add backward-compatible providers, checks, flags, report
  fields, or Helm values.
- Major releases are reserved for breaking public contracts.

The v1 JSON result schema is documented in [REPORTING.md](REPORTING.md).
Support windows, stable contracts, and deprecation rules are documented in
[SUPPORT.md](SUPPORT.md).
