# Installation

`restore-drill` is a single static binary. Install it however suits your
environment, then verify it runs with `restore-drill version`.

## go install

```bash
go install github.com/RamazanKara/restore-drill/cmd/restore-drill@latest
```

## Container image

Release images are published to GitHub Container Registry:

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

Release images and checksum artifacts are signed with keyless Sigstore/Cosign
from GitHub Actions. After installing [cosign](https://docs.sigstore.dev/), verify
the container image:

```bash
cosign verify \
  --certificate-identity-regexp 'https://github.com/RamazanKara/restore-drill/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/ramazankara/restore-drill:latest
```

Checksums are published with a cosign bundle (`checksums.txt.bundle`) and a
build-provenance attestation on every release.

## Requirements

- A container runtime: **Docker** (local/CI) or **Kubernetes** (Helm chart).
- Each drill's **restore target image** must contain the database runtime, its
  client tools, and the selected backup tool. Local/S3 staging also needs `tar`
  in that image. `restore-drill doctor` and preflight checks report missing
  commands early.

## See also

- [Quick start](quickstart.md) — run your first drill.
- [CLI reference](../reference/cli.md) — all commands and flags.
