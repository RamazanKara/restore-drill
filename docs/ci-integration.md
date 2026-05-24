# CI/CD Integration

Run `restore-drill` in your CI/CD pipeline to continuously prove backup recovery works.
A non-zero exit code signals a failed drill, which can gate deployments.

## GitHub Actions

```yaml
name: Backup Drill
on:
  schedule:
    - cron: '0 6 * * *'  # daily at 06:00 UTC
  workflow_dispatch: {}

jobs:
  restore-drill:
    runs-on: ubuntu-latest
    services:
      docker:
        image: docker:dind
        options: --privileged
    steps:
      - uses: actions/checkout@v4

      - name: Install restore-drill
        run: |
          curl -sSfL https://github.com/RamazanKara/restore-drill/releases/latest/download/restore-drill_linux_amd64.tar.gz | tar xz
          sudo mv restore-drill /usr/local/bin/

      - name: Run backup verification
        run: restore-drill run --config drill.yaml --runtime docker --format json
        env:
          AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}

      - name: Generate compliance report
        if: always()
        run: restore-drill report --last 90 --output report.html

      - name: Upload report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: compliance-report
          path: report.html
```

## GitLab CI

```yaml
backup-drill:
  stage: test
  image: docker:latest
  services:
    - docker:dind
  variables:
    DOCKER_HOST: tcp://docker:2375
  before_script:
    - apk add --no-cache curl
    - curl -sSfL https://github.com/RamazanKara/restore-drill/releases/latest/download/restore-drill_linux_amd64.tar.gz | tar xz
    - mv restore-drill /usr/local/bin/
  script:
    - restore-drill run --config drill.yaml --runtime docker
  rules:
    - if: $CI_PIPELINE_SOURCE == "schedule"
    - if: $CI_PIPELINE_SOURCE == "web"
  artifacts:
    when: always
    paths:
      - report.html
```

## ArgoCD Post-Sync Hook

Run a drill after every deployment to verify backups are still restorable:

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
      containers:
        - name: drill
          image: ghcr.io/ramazankara/restore-drill:latest
          args: ["run", "--config", "/config/drill.yaml", "--runtime", "kubernetes"]
          volumeMounts:
            - name: config
              mountPath: /config
          env:
            - name: AWS_REGION
              value: eu-central-1
          envFrom:
            - secretRef:
                name: backup-credentials
      volumes:
        - name: config
          configMap:
            name: restore-drill-config
      restartPolicy: Never
```

## Kubernetes CronJob

Schedule drills as a Kubernetes CronJob (included in the Helm chart):

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: restore-drill
spec:
  schedule: "0 4 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: restore-drill
          containers:
            - name: drill
              image: ghcr.io/ramazankara/restore-drill:latest
              args: ["run", "--config", "/config/drill.yaml", "--runtime", "kubernetes"]
              volumeMounts:
                - name: config
                  mountPath: /config
          volumes:
            - name: config
              configMap:
                name: restore-drill-config
          restartPolicy: OnFailure
```

## One-Shot Incident Mode

During an incident, verify a specific point-in-time recovery:

```bash
# Restore to a specific timestamp and keep the container running for inspection
restore-drill run \
  --config drill.yaml \
  --target "2024-01-15T10:30:00Z" \
  --no-cleanup

# Connect to the running container
docker exec -it <container-id> psql -U postgres
```

The `--target` flag overrides the PITR target for all drills in the config.
The `--no-cleanup` flag keeps containers running so you can inspect the restored data.

## Compliance Reports

Generate compliance reports aggregating drill history:

```bash
# HTML report for the last 90 days (default)
restore-drill report --last 90 --output compliance-report.html

# JSON format for programmatic consumption
restore-drill report --format json --last 30
```

Reports map drill outcomes to compliance controls:

| Framework       | Control      | Description                              |
|-----------------|--------------|------------------------------------------|
| ISO 27001:2022  | A.8.13       | Information backup — restore testing     |
| NIS2 Directive  | Art. 21(2)(c)| Business continuity — backup verification|
| BSI C5:2020     | OPS-04       | Data backup concept — regular restore tests |
| BSI C5:2020     | OPS-05       | Recovery time objectives                 |
| SOC 2           | A1.2         | Recovery testing                         |

HTML reports also include per-check failure evidence with the expected value, actual value, and provider error so failed drills can be reviewed without digging through raw job logs first.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0    | All drills passed |
| 1    | One or more drills failed validation |

Use the exit code in CI to gate deployments or trigger alerts.
