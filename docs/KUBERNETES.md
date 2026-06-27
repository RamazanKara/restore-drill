# Kubernetes and Helm

The Helm chart deploys restore-drill as a CronJob. The CronJob runs
`restore-drill run --runtime=kubernetes`, and each drill creates a short-lived
restore target pod.

Use the chart when restore verification should run on a cluster schedule and
the restore target needs access to in-cluster secrets, service accounts, network
policy, or object-storage identity. Use the Docker runtime locally or in CI when
you only need a quick restore proof outside the cluster.

## Install

Use inline config for simple deployments:

```bash
helm install restore-drill deploy/helm/restore-drill \
  --namespace restore-drill \
  --create-namespace \
  --set-file config.inline=drill.yaml
```

Or reference an existing ConfigMap that contains `drill.yaml`:

```yaml
config:
  existingConfigMap: restore-drill-config
```

Helm rendering fails fast when neither `config.inline` nor
`config.existingConfigMap` is set.

## Runtime behavior

- The CronJob pod runs the restore-drill CLI.
- Restore target pods are created in the release namespace by default.
- `runtime.namespace` can place restore target pods in another namespace.
- `runtime.serviceAccountName`, `runtime.imagePullSecrets`,
  `runtime.podLabels`, and `runtime.podAnnotations` apply to restore target
  pods.
- Top-level `serviceAccount`, `imagePullSecrets`, `podLabels`, and
  `podAnnotations` apply to the CronJob pod.
- `--no-cleanup` keeps the target pod for manual inspection and records the pod
  ID in state and JSON output.

## Values that matter most

Start with these values before tuning the rest of
[values.yaml](../deploy/helm/restore-drill/values.yaml):

```yaml
schedule: "0 3 * * *"
concurrencyPolicy: Forbid
activeDeadlineSeconds: 3600
backoffLimit: 1

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
  egress: []
```

Keep `concurrencyPolicy: Forbid` unless restore targets are independent and the
cluster has enough resource headroom for overlapping drills.

## RBAC

With `rbac.create=true`, the chart creates a namespace-scoped Role and
RoleBinding for:

- pod create, get, list, watch, and delete
- pod exec
- pod logs

If `runtime.namespace` differs from the release namespace, the Role and
RoleBinding are created in `runtime.namespace`, and the subject references the
CronJob service account in the release namespace.

## Secrets and environment

Prefer environment variables sourced from Kubernetes Secrets or workload
identity. The config supports `${VAR}` interpolation before YAML parsing:

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

Use `extraVolumes` and `extraVolumeMounts` for mounted backup files,
certificates, or provider credentials.

Do not put backup credentials directly in `config.inline`; rendered Helm output
is often stored in CI logs, GitOps diffs, or release history.

## Restore target images

The restore target image comes from each drill's `restore.container.image`, not
from the chart image. It must contain the provider runtime and selected backup
tool:

- PostgreSQL: `psql`, `pg_isready`, plus `pgbackrest`, `pg_restore`, or
  `wal-g`/`walg` when selected
- MySQL/MariaDB: `mysql` or `mariadb`, `mysqladmin` or `mariadb-admin`, plus
  `xtrabackup` or `mariabackup` when selected
- Redis: `redis-server` and `redis-cli`

Archive restores also need archive utilities such as `tar`, `gzip`, and
`xbstream` depending on the backup format.
Local/S3 staging requires `tar` because backup artifacts are copied into restore
target pods as tar streams.

## Network policy

Enable `networkPolicy.enabled` only after adding egress for everything the
CronJob and restore targets need:

- DNS
- object storage or backup repositories
- Pushgateway
- webhook endpoints
- registry endpoints if images are pulled at runtime

The default NetworkPolicy template allows DNS and appends
`networkPolicy.egress`.

## Metrics

restore-drill does not expose a long-lived metrics HTTP service in Kubernetes.
The Helm chart runs it as a CronJob, and each job exits after pushing run
results to the configured Prometheus Pushgateway.

With Prometheus Operator, attach a `ServiceMonitor` to the Pushgateway service,
not to restore-drill CronJob pods. A CronJob does not provide a stable
restore-drill endpoint to scrape, so a direct restore-drill `ServiceMonitor`
would be noisy at best and usually empty.

Prometheus alert examples are documented in [REPORTING.md](REPORTING.md).

## Troubleshooting

Run a one-shot drill and retain the pod for inspection:

```bash
restore-drill run \
  --runtime kubernetes \
  --kube-namespace restore-drill \
  --config drill.yaml \
  --no-cleanup \
  --format json
```

Then inspect the retained pod:

```bash
kubectl get pods -n restore-drill -l restore-drill/ephemeral=true
kubectl logs -n restore-drill <pod-name>
kubectl exec -n restore-drill -it <pod-name> -- sh
```

Delete retained pods after inspection:

```bash
kubectl delete pod -n restore-drill -l restore-drill/ephemeral=true
```
