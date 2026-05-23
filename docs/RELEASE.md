# Release Process

restore-drill releases are tag driven. The canonical module is `github.com/RamazanKara/restore-drill`, and container images are published to `ghcr.io/ramazankara/restore-drill`.

## Local release checks

Install the local release toolchain first:

- Go
- Docker with Buildx
- Helm
- GoReleaser
- Syft, used by GoReleaser for SBOM generation
- kind and kubectl for Kubernetes smoke tests

```bash
make verify
make test-k8s
RESTORE_DRILL_INTEGRATION=1 go test -race -count=1 -timeout=10m ./test/integration/...
goreleaser release --snapshot --clean --skip=publish
cd dist && sha256sum -c checksums.txt
```

GoReleaser currently warns that its stable `dockers` and `docker_manifests`
keys will eventually be replaced by `dockers_v2`. `dockers_v2` is still marked
experimental in GoReleaser 2.15.4, so release builds intentionally keep the
stable Docker path until the replacement is production-ready.

## Tagging

```bash
git tag v0.2.0
git push origin v0.2.0
```

The release workflow builds binaries, checksums, SBOMs, and multi-architecture Docker images.

## Before publishing v0.2.0

- README, examples, Helm chart, and release links use the `RamazanKara/restore-drill` namespace.
- `make verify` passes in CI.
- Docker integration tests pass for advertised provider paths.
- Helm lint/template passes.
- GoReleaser snapshot passes.
- No credentials, real backup data, or generated binaries are committed.
