# Installation

`restore-drill` is a Go command-line tool. Install it however suits your
environment, then verify it runs with `restore-drill version`.

## go install

```bash
go install github.com/RamazanKara/restore-drill/cmd/restore-drill@latest
```

This source install reports `dev` in `restore-drill version`. Release binaries
and `make build` embed version metadata.

## Container image

The release workflow targets GitHub Container Registry:

```bash
docker pull ghcr.io/ramazankara/restore-drill:latest
```

Pin a specific version (e.g. `:1.3.0`) for reproducible runs.

## From source

```bash
git clone https://github.com/RamazanKara/restore-drill
cd restore-drill
make build      # produces ./bin/restore-drill
```

## Verify release signatures

The release workflow includes keyless Sigstore/Cosign signing.
After installing [cosign](https://docs.sigstore.dev/), verify
the container image:

```bash
cosign verify \
  --certificate-identity-regexp 'https://github.com/RamazanKara/restore-drill/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/ramazankara/restore-drill:latest
```

The workflow also generates a checksum bundle (`checksums.txt.bundle`) and
build-provenance attestation. Verify the artifacts for the release you use.

## Requirements

- Source builds require the Go version in `go.mod` or newer. Make targets need
  GNU Make and a POSIX shell; race tests also need a C compiler.
- A container runtime: **Docker** (local/CI) or **Kubernetes** (Helm chart).
- Each drill's **restore target image** must contain the database runtime and
  the tools needed for the selected restore flow. Local/S3 staging also needs `tar`
  in that image. Provider preflight checks inspect tools after creating the
  target; `doctor` checks local configuration, runtime access, paths, and local
  development tools without starting a restore target.

## See also

- [Quick start](quickstart.md) — run your first drill.
- [CLI reference](../reference/cli.md) — all commands and flags.
