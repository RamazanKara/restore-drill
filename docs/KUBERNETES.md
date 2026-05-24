# Kubernetes and Helm

The Helm chart deploys restore-drill as a CronJob and runs the CLI with the Kubernetes runtime.

## Install

```bash
helm install restore-drill deploy/helm/restore-drill \
  --namespace restore-drill \
  --create-namespace \
  --set-file config.inline=drill.yaml
```

The chart requires either `config.inline` or `config.existingConfigMap`; Helm rendering fails fast if neither is set.

For production, prefer secret-backed environment variables for backup credentials:

```yaml
env:
  - name: AWS_ACCESS_KEY_ID
    valueFrom:
      secretKeyRef:
        name: backup-credentials
        key: access-key-id
  - name: AWS_SECRET_ACCESS_KEY
    valueFrom:
      secretKeyRef:
        name: backup-credentials
        key: secret-access-key
```

## Runtime behavior

- The CronJob pod runs `restore-drill run --runtime=kubernetes`.
- Restore targets are short-lived pods in the release namespace by default.
- The chart creates a Role and RoleBinding for pod create/get/list/watch/delete, pod exec, and pod logs.
- `runtime.namespace` can override where ephemeral restore pods are created.
- `runtime.serviceAccountName`, `runtime.imagePullSecrets`, `runtime.podLabels`, and `runtime.podAnnotations` apply to the ephemeral restore pods created for each drill.
- `--no-cleanup` keeps the target pod for manual inspection and records its ID in state/report output.

## Values that matter most

```yaml
schedule: "0 3 * * *"
runtime:
  mode: kubernetes
  namespace: ""
  serviceAccountName: ""
  imagePullSecrets: []
  podLabels: {}
  podAnnotations: {}
config:
  inline: ""
  existingConfigMap: ""
rbac:
  create: true
networkPolicy:
  enabled: false
```

Enable `networkPolicy` only after adding egress rules for DNS, backup storage, and Pushgateway/webhook targets.

Top-level `serviceAccount`, `imagePullSecrets`, `podLabels`, and `podAnnotations` configure the CronJob pod that runs restore-drill. The nested `runtime.*` values configure the short-lived database restore pods that restore-drill creates.

## Metrics

restore-drill does not expose a long-lived metrics HTTP service in Kubernetes. The Helm chart runs it as a CronJob, and each job exits after pushing run results to the configured Prometheus Pushgateway.

If your cluster uses Prometheus Operator, enable or create a `ServiceMonitor` for the Pushgateway service itself. A `ServiceMonitor` for the restore-drill CronJob would not have a stable restore-drill endpoint to scrape.

## Image requirements

The restore target image comes from each drill's `restore.container.image`, not from the chart image. It must contain the provider runtime and selected backup tool:

- PostgreSQL: `psql`, `pg_isready`, plus `pgbackrest`, `pg_restore`, or `wal-g` when selected
- MySQL/MariaDB: `mysql`, `mysqladmin`, plus `xtrabackup` or `mariabackup` when selected
- Redis: `redis-server`, `redis-cli`
