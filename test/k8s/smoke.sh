#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${KIND_CLUSTER_NAME:-restore-drill-ci}"
NODE_IMAGE="${KIND_NODE_IMAGE:-kindest/node:v1.31.4}"
NAMESPACE="${RESTORE_DRILL_K8S_NAMESPACE:-default}"
HELM_NAMESPACE="${RESTORE_DRILL_HELM_NAMESPACE:-restore-drill}"
KEEP_CLUSTER="${KEEP_KIND_CLUSTER:-0}"

created_cluster=0
created_helm_namespace=0
if ! kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"; then
  kind create cluster --name "${CLUSTER_NAME}" --image "${NODE_IMAGE}" --wait 120s
  created_cluster=1
fi

cleanup() {
  if [[ "${created_helm_namespace}" == "1" ]]; then
    kubectl delete namespace "${HELM_NAMESPACE}" --ignore-not-found >/dev/null 2>&1 || true
  fi
  if [[ "${created_cluster}" == "1" && "${KEEP_CLUSTER}" != "1" ]]; then
    kind delete cluster --name "${CLUSTER_NAME}"
  fi
}
trap cleanup EXIT

kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"; cleanup' EXIT

: > "${tmp_dir}/appendonly.aof"
cat > "${tmp_dir}/drill.yaml" <<EOF
drills:
  - name: k8s-redis-aof
    provider: redis
    backup:
      tool: aof
      source: ${tmp_dir}/appendonly.aof
    restore:
      timeout: 3m
      container:
        image: redis:7-alpine
    checks:
      - name: ping
        type: query
        sql: "PING"
        expect: 'contains "PONG"'
EOF

go run ./cmd/restore-drill run \
  --runtime kubernetes \
  --kube-namespace "${NAMESPACE}" \
  --config "${tmp_dir}/drill.yaml" \
  --format json

kubectl get pods -n "${NAMESPACE}" -l restore-drill/ephemeral=true --no-headers 2>/dev/null | grep -v Terminating && {
  echo "restore-drill ephemeral pods remain after smoke test" >&2
  exit 1
}

retained_output="${tmp_dir}/retained.json"
go run ./cmd/restore-drill run \
  --runtime kubernetes \
  --kube-namespace "${NAMESPACE}" \
  --config "${tmp_dir}/drill.yaml" \
  --format json \
  --no-cleanup >"${retained_output}"

grep -q '"cleanup_skipped": true' "${retained_output}" || {
  echo "retained Kubernetes run did not report cleanup_skipped" >&2
  cat "${retained_output}" >&2
  exit 1
}

retained_pod="$(kubectl get pods -n "${NAMESPACE}" -l restore-drill/ephemeral=true -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)"
if [[ -z "${retained_pod}" ]]; then
  echo "retained Kubernetes run did not leave an ephemeral pod" >&2
  exit 1
fi

kubectl delete pod -n "${NAMESPACE}" -l restore-drill/ephemeral=true --wait=true >/dev/null
kubectl get pods -n "${NAMESPACE}" -l restore-drill/ephemeral=true --no-headers 2>/dev/null | grep -v Terminating && {
  echo "restore-drill retained pods remain after explicit cleanup" >&2
  exit 1
}

if ! kubectl get namespace "${HELM_NAMESPACE}" >/dev/null 2>&1; then
  kubectl create namespace "${HELM_NAMESPACE}" >/dev/null
  created_helm_namespace=1
fi
helm install restore-drill deploy/helm/restore-drill \
  --namespace "${HELM_NAMESPACE}" \
  --set-file config.inline=examples/redis-rdb.yaml \
  -f test/k8s/helm-runtime-options.yaml \
  --dry-run=server >/dev/null
