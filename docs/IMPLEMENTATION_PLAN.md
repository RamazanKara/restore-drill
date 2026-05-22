# Implementation Plan

## Summary

Build `restore-drill` in 4 phases over ~30 days. Each phase produces a working, shippable increment.

---

## Phase 1: Core engine + PostgreSQL provider (Days 1–10)

**Goal:** A working CLI that restores a PostgreSQL backup and validates it.

### Tasks

1. **Project bootstrap**
   - Go module init (`github.com/fluentorbit/restore-drill`)
   - Makefile (build, test, lint, release)
   - CI workflow (GitHub Actions: test, lint, goreleaser)
   - Cobra CLI scaffold (`run`, `status`, `version` commands)

2. **Config parser**
   - YAML config struct with validation
   - Support for environment variable interpolation in secrets (`${PG_PASSWORD}`)
   - Config loading from file path or stdin

3. **Container runtime: Docker adapter**
   - Implement `Runtime` interface using Docker SDK
   - Container create, exec, copy-to, destroy, logs
   - Health-check polling (wait for port ready)
   - Resource limits (memory, CPU)
   - Automatic cleanup on context cancellation

4. **PostgreSQL provider**
   - pgBackRest restore (S3 source)
   - pg_dump/pg_restore (S3 or local file)
   - WAL-G restore (S3 source)
   - Connection establishment after restore
   - Provider-specific validation (extensions, replication slots)

5. **Validator: core check types**
   - `query` — execute SQL, compare result against expression
   - `schema` — check migration version table
   - `freshness` — compare max timestamp column against wall clock
   - Expression evaluator: `> N`, `< N`, `== N`, `age < Xh`, `contains "..."`, `matches_migration_version >= N`

6. **Reporter: stdout**
   - Colored table output (pass/fail, durations)
   - Exit code 0 on success, 1 on failure

### Definition of done

```bash
restore-drill run --config examples/postgres-pgbackrest.yaml
# → Pulls backup from S3
# → Spins up postgres:16 container
# → Restores via pgBackRest
# → Runs 3 validation queries
# → Prints results table
# → Destroys container
# → Exits 0/1
```

### Tests

- Unit tests for config parsing, expression evaluation, validator logic
- Integration test with Docker: restore a pg_dump fixture, validate

---

## Phase 2: Metrics, reporting, and additional providers (Days 11–18)

**Goal:** Production-grade observability and MySQL/Redis support.

### Tasks

7. **Prometheus metrics**
   - Pushgateway client
   - All metrics from README (duration, backup age, validation status, etc.)
   - Label management (drill name, provider, environment)

8. **Reporter: JSON and HTML**
   - JSON report per drill execution (machine-readable)
   - HTML report with summary table, per-check details, trend chart placeholder
   - Report archival with configurable retention

9. **Reporter: webhook**
   - POST JSON payload to configured URL
   - Slack block format (optional)
   - Retry with backoff on 5xx

10. **MySQL provider**
    - mysqldump restore (from S3/local gzip)
    - xtrabackup restore
    - mariabackup restore
    - Connection and validation

11. **Redis provider**
    - RDB file restore (copy into container, start redis-server)
    - Key count validation
    - Key sample validation (pattern-based existence check)
    - TTL validation (keys haven't all expired)

12. **S3 provider**
    - rclone sync from source to ephemeral MinIO container
    - Object count and size validation
    - Specific object existence checks

### Definition of done

- `restore-drill run` pushes metrics to Pushgateway after each drill
- HTML report generates a self-contained file with drill history
- MySQL and Redis drills work end-to-end

### Tests

- Unit tests for metric registration, report generation
- Integration tests for MySQL and Redis providers (Docker-based)

---

## Phase 3: Kubernetes runtime + Helm chart (Days 19–25)

**Goal:** Deploy as a CronJob in production clusters.

### Tasks

13. **Kubernetes runtime adapter**
    - Create Job (child pod) for the ephemeral database
    - Exec into pod for restore commands
    - Port-forward or in-cluster networking for validation queries
    - Cleanup: delete Job + PVC on completion
    - Respect resource quotas and node selectors from config

14. **etcd provider**
    - etcdctl snapshot restore
    - Member list validation
    - Key count and prefix checks

15. **Helm chart**
    - CronJob template per drill
    - ConfigMap for drill config
    - Secret references for backup credentials
    - ServiceAccount with minimal RBAC
    - NetworkPolicy (allow egress to S3 + Pushgateway only)
    - Values: schedule, image tag, resources, drills list

16. **Multi-drill orchestration**
    - Run multiple drills from one config (sequential by default)
    - `--parallel` flag for independent drills
    - Per-drill timeout enforcement
    - Aggregate exit code (fail if any drill fails)

17. **`status` command**
    - Query Pushgateway or local state file for last drill results
    - Show table of all configured drills with last status/duration/timestamp

### Definition of done

```bash
helm install restore-drill deploy/helm/ \
  --namespace restore-drill \
  --set-file config=drill.yaml
# → CronJob created
# → Next scheduled run creates child Job, restores, validates, pushes metrics
```

### Tests

- Integration test with kind cluster (create Job, validate lifecycle)
- Helm template tests (helm unittest)

---

## Phase 4: Polish, compliance, and release (Days 26–30)

**Goal:** Public release with documentation, examples, and compliance output.

### Tasks

18. **Compliance report generator**
    - `restore-drill report` subcommand
    - Aggregate historical results (from local JSON or Prometheus query)
    - HTML output with: executive summary, drill history table, RTO trend chart, RPO measurements
    - PDF output (via wkhtmltopdf or headless Chrome, optional)
    - Map to compliance controls (NIS2 Art. 21, ISO 27001 A.12.3, BSI C5 OPS-04)

19. **Example configurations**
    - `examples/postgres-pgbackrest.yaml`
    - `examples/postgres-waldump.yaml`
    - `examples/mysql-xtrabackup.yaml`
    - `examples/redis-rdb.yaml`
    - `examples/s3-rclone.yaml`
    - `examples/etcd-snapshot.yaml`
    - `examples/multi-drill.yaml`

20. **One-shot incident mode**
    - `--target` flag for PITR timestamp
    - `--no-cleanup` flag to keep container running
    - Print connection string for manual inspection

21. **CI/CD integration docs**
    - GitHub Actions example (run drill in CI, fail on regression)
    - GitLab CI example
    - ArgoCD hook example (post-sync drill)

22. **goreleaser config**
    - Multi-arch binaries (linux/darwin, amd64/arm64)
    - Docker image (multi-arch)
    - Homebrew tap
    - SBOM generation

23. **README final polish**
    - GIF/asciicast of a successful drill
    - Badges (CI, coverage, release, Go report)
    - "Used by" section placeholder

### Definition of done

- `v0.1.0` tag pushed
- Docker image published to GHCR
- Helm chart installable
- All examples work against fixture backups
- Compliance report generates valid HTML

---

## Tech stack decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Go | Single binary, fast startup, good Docker/K8s SDKs |
| CLI framework | Cobra | Standard, well-maintained |
| Config format | YAML | Familiar for K8s operators |
| Container SDK | Docker SDK for Go | Direct API, no shell-out |
| Kubernetes SDK | client-go | Official, typed |
| Metrics | prometheus/client_golang | Standard |
| HTML reports | Go html/template | No JS deps, self-contained output |
| Build/release | goreleaser | Multi-arch, Homebrew, Docker |
| Testing | testify + testcontainers-go | Real integration tests |

## Risk mitigation

| Risk | Mitigation |
|------|-----------|
| pgBackRest restore is complex | Start with pg_dump (simpler), add pgBackRest as second path |
| Docker-in-Docker in K8s | Use Kubernetes Job as runtime (no DinD needed) |
| S3 credential handling | Support IRSA (EKS), workload identity, and explicit keys |
| Large backup download time | Add progress reporting, configurable timeout, pre-pull support |
| Container image pull failures | Retry with backoff, support private registries via imagePullSecrets |

## Success metrics (for the OSS project itself)

- Week 1: Working PostgreSQL drill, demo GIF, initial blog post
- Week 2: 3+ providers working, Helm chart deployable
- Week 4: First external contributor or user report
- Month 2: 100+ GitHub stars
- Month 3: Featured in a "self-hosted" or "backup" community newsletter
