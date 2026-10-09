# Release Process

Release preparation is local while GitHub Actions is unavailable. The single
workflow performs verification; it does not publish releases or documentation.
The canonical module is
`github.com/RamazanKara/restore-drill`, and container images are published to
`ghcr.io/ramazankara/restore-drill`.

This page is for maintainers cutting a release. Users looking for installation
options should start with the root [README](../getting-started/installation.md).

## Release toolchain

Install these tools before running the full local release gate:

- Go
- GNU Make, a POSIX shell, and `sha256sum` (Git Bash supplies the latter two on Windows)
- golangci-lint and staticcheck at the versions pinned in `.github/workflows/ci.yml`
- Docker with Buildx
- Helm
- GoReleaser
- Syft, used by GoReleaser for SBOM generation
- Cosign, used for keyless release signature verification and local signing checks
- govulncheck, or network access for the `go run` fallback used by `make vuln`
- kind and kubectl for Kubernetes smoke tests

## Local release gate

Run the core gate on Linux/macOS or Windows with GNU Make and Git Bash on `PATH`:

```bash
make build fmt-check vet test-unit test-fuzz lint staticcheck vuln check-examples
```

`test-unit` uses `-race` only when `go env CGO_ENABLED` is `1`; a C compiler is
then required. With CGO disabled, the unit and fuzz tests still run.

Build an unsigned native binary locally, without Docker or GoReleaser:

```bash
make local-release VERSION=vX.Y.Z
cd dist
sha256sum -c SHA256SUMS
```

This writes `restore-drill` (`restore-drill.exe` on Windows) and `SHA256SUMS`.
The binary uses `-trimpath`, embeds version/commit/UTC build time, and disables
CGO. Nothing is tagged, signed, uploaded, or published. From the repository root,
verify the generated checksum in PowerShell with:

```powershell
$expected = (Get-Content dist/SHA256SUMS).Split(' ')[0]
$actual = (Get-FileHash dist/restore-drill.exe -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $expected) { throw 'SHA256 mismatch' }
```

For full runtime and multi-platform release validation, also run:

```bash
make verify
make docker-smoke
make test-k8s
make test-integration
goreleaser release --snapshot --clean --skip=publish
cd dist && sha256sum -c checksums.txt
```

`make verify` runs build, formatting checks, vet, unit and fuzz tests, lint,
staticcheck, govulncheck with
the reviewed Docker/Moby allowlist, example validation, Helm lint/template, and
`goreleaser check`.

As of 2026-10-09, the security gate is blocked by the existing Docker SDK:
govulncheck reports [GO-2026-4887](https://pkg.go.dev/vuln/GO-2026-4887) and
[GO-2026-4883](https://pkg.go.dev/vuln/GO-2026-4883), both with no fixed version
listed for `github.com/docker/docker`. The existing allowlist expired on
2026-09-30. A locally built artifact is not a security-cleared release until
this gate is resolved.

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

The GoReleaser configuration builds:

- Linux and Darwin binaries for amd64 and arm64
- archive checksums
- SBOMs
- multi-architecture GHCR images
- `latest` and versioned container tags

Tagging alone does not publish anything while Actions is unavailable. Signing
and publication are separate maintainer steps after all release gates pass.

## Verification

Verify checksums after downloading release archives:

```bash
sha256sum -c checksums.txt
```

For older releases signed by the former GitHub Actions release workflow, verify
the GHCR image signature:

```bash
cosign verify \
  --certificate-identity-regexp 'https://github.com/RamazanKara/restore-drill/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/ramazankara/restore-drill:<version>
```

Verify the checksum bundle:

```bash
cosign verify-blob \
  --bundle checksums.txt.bundle \
  --certificate-identity-regexp 'https://github.com/RamazanKara/restore-drill/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

Pin production images by digest after verification:

```bash
docker pull ghcr.io/ramazankara/restore-drill:<version>
docker inspect --format='{{index .RepoDigests 0}}' ghcr.io/ramazankara/restore-drill:<version>
```

## Release hygiene

Before publishing a tag:

- README, examples, Helm chart, and release links use the
  `RamazanKara/restore-drill` namespace.
- GHCR references use lowercase `ghcr.io/ramazankara/restore-drill`.
- Roadmap items are clearly marked as roadmap, not implied GA behavior.
- `CHANGELOG.md` has an entry for the release.
- Provider claims in README and docs are backed by tests or marked as roadmap.
- Release assets include checksums and SBOMs.
- Release images and checksums are signed, and checksums have provenance
  attestations.
- No credentials, real backup data, generated binaries, or release artifacts are
  committed.

## Versioning

restore-drill follows semantic versioning:

- Patch releases fix bugs without changing public YAML, CLI, metrics, or JSON
  contracts.
- Minor releases add backward-compatible providers, checks, flags, report
  fields, or Helm values.
- Major releases are reserved for breaking public contracts.

The v1 JSON result schema is documented in [REPORTING.md](../reference/reporting.md).
Support windows, stable contracts, and deprecation rules are documented in
[SUPPORT.md](support.md).
