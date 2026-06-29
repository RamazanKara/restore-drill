# CI/CD and Scheduled Drills

Run `restore-drill` on a schedule or after deployments to continuously prove
backup recovery works. A non-zero exit code means one or more drills failed and
can gate a pipeline.

Use Docker runtime jobs for portable CI restore proofs. Use the Kubernetes
runtime when the drill must run inside the cluster with namespace-scoped RBAC,
Secrets, service accounts, and NetworkPolicy.

## GitHub Actions

Docker is available on `ubuntu-latest`, so the simplest workflow uses the Docker
runtime directly:

```yaml
name: Backup Drill

on:
  schedule:
    - cron: "0 6 * * *"
  workflow_dispatch: {}

jobs:
  restore-drill:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: true

      - name: Install restore-drill
        run: go install github.com/RamazanKara/restore-drill/cmd/restore-drill@latest

      - name: Run backup verification
        run: restore-drill run --config drill.yaml --runtime docker --format json
        env:
          AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          RESTORE_DRILL_WEBHOOK_TOKEN: ${{ secrets.RESTORE_DRILL_WEBHOOK_TOKEN }}

      - name: Generate restore evidence report
        if: always()
        run: restore-drill report --last 90 --output restore-evidence.html

      - name: Upload report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: restore-drill-report
          path: restore-evidence.html
```

For scheduled evidence files from every run, configure:

```yaml
reporting:
  format: [json, html]
  output: ./reports/
  retention: 90d
```

Then upload `reports/**` as an artifact.

The repository Kubernetes smoke test defaults to `kindest/node:v1.36.1` with
kind `v0.32.0`. That node image requires Docker cgroup v2. On older local
hosts that still report Docker cgroup v1, keep CI on the default image but run
local smoke checks with a compatible override such as
`KIND_NODE_IMAGE=kindest/node:v1.34.0 make test-k8s`.

## GitLab CI

```yaml
backup-drill:
  stage: test
  image: docker:latest
  services:
    - docker:dind
  variables:
    DOCKER_HOST: tcp://docker:2375
    DOCKER_TLS_CERTDIR: ""
  before_script:
    - apk add --no-cache go git
    - go install github.com/RamazanKara/restore-drill/cmd/restore-drill@latest
    - export PATH="$(go env GOPATH)/bin:$PATH"
  script:
    - restore-drill run --config drill.yaml --runtime docker --format json
    - restore-drill report --last 90 --output restore-evidence.html
  rules:
    - if: $CI_PIPELINE_SOURCE == "schedule"
    - if: $CI_PIPELINE_SOURCE == "web"
  artifacts:
    when: always
    paths:
      - restore-evidence.html
      - reports/
```

## ArgoCD post-sync hook

Run a one-shot Kubernetes runtime drill after a deployment:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: restore-drill-postsync
  annotations:
    argocd.argoproj.io/hook: PostSync
    argocd.argoproj.io/hook-delete-policy: HookSucceeded
spec:
  backoffLimit: 1
  template:
    spec:
      serviceAccountName: restore-drill
      restartPolicy: Never
      containers:
        - name: drill
          image: ghcr.io/ramazankara/restore-drill:latest
          args: ["run", "--config", "/config/drill.yaml", "--runtime", "kubernetes"]
          volumeMounts:
            - name: config
              mountPath: /config
          envFrom:
            - secretRef:
                name: backup-credentials
      volumes:
        - name: config
          configMap:
            name: restore-drill-config
```

For routine Kubernetes schedules, prefer the Helm chart documented in
[KUBERNETES.md](kubernetes.md).

## Incident mode

During an incident, verify a specific point-in-time restore and retain the
target for inspection:

```bash
restore-drill run \
  --config drill.yaml \
  --runtime docker \
  --target "2026-05-20T14:30:00Z" \
  --no-cleanup \
  --format json
```

The `--target` flag overrides `restore.target` for all drills in the config.
The `--no-cleanup` flag keeps the target running and includes connection details
in JSON/state output.

Docker inspection example:

```bash
docker exec -it <container-id> psql -U postgres
```

Kubernetes inspection example:

```bash
kubectl exec -n restore-drill -it <pod-name> -- sh
```

## Reports

Generate restore evidence reports from local history:

```bash
restore-drill report --last 90 --output restore-evidence.html
restore-drill report --format json --last 30
```

HTML reports include per-check failure evidence with expected values, actual
values, and provider errors. The report contract is documented in
[REPORTING.md](../reference/reporting.md).

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | All drills passed. |
| 1 | One or more drills failed validation, restore, reporting, or configuration. |

Use the exit code to gate deployments or trigger alerts.

## See also

- [CLI reference](../reference/cli.md)
- [Incident response](incident-response.md)
- [Reporting & alerts](../reference/reporting.md)
