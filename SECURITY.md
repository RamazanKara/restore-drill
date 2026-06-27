# Security Policy

restore-drill handles backup locations, restored production-like data, and
secret-bearing environment variables. Please report security issues privately.

## Supported versions

Security fixes are provided for the latest released minor version.

## Reporting a vulnerability

Please do not open a public GitHub issue for a suspected vulnerability. Send a
private report to the repository owner through GitHub security advisories or the
contact channel listed on the GitHub profile.

Include:

- affected version or commit
- reproduction steps
- expected impact
- any relevant logs with secrets removed

## Secret handling expectations

- Do not put credentials directly in drill config.
- Use environment interpolation, Kubernetes Secrets, workload identity, or
  cloud-native credentials.
- Redact backup URLs and command output before sharing logs.
- Treat `--no-cleanup` restore targets and generated reports as sensitive when
  they may contain restored production-like data.

## Vulnerability scanning

CI and the scheduled Security workflow run `make vuln`, which executes govulncheck through
`scripts/govulncheck.sh`. Fixable Go standard-library and module
vulnerabilities fail the build.

The only allowed findings are reviewed no-fixed-version Docker/Moby advisories
listed in `.govulncheck.allowlist`. Each entry has an expiry date and must be
rechecked before release. restore-drill avoids Docker's archive upload endpoint
for backup staging and streams tar data through exec instead.
