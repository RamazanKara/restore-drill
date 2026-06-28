#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${KIND_CLUSTER_NAME:-restore-drill-ci}"
NODE_IMAGE="${KIND_NODE_IMAGE:-kindest/node:v1.36.1}"
NAMESPACE="${RESTORE_DRILL_K8S_NAMESPACE:-default}"
HELM_NAMESPACE="${RESTORE_DRILL_HELM_NAMESPACE:-restore-drill}"
KEEP_CLUSTER="${KEEP_KIND_CLUSTER:-0}"

require_kind_for_node_image() {
  if [[ ! "${NODE_IMAGE}" =~ ^kindest/node:v1\.36\. ]]; then
    return
  fi

  local current required oldest
  current="$(kind version | awk '{print $2}')"
  required="v0.32.0"
  oldest="$(printf '%s\n%s\n' "${required}" "${current}" | sort -V | head -n1)"
  if [[ -z "${current}" || "${oldest}" != "${required}" ]]; then
    echo "${NODE_IMAGE} requires kind ${required} or newer; found ${current:-unknown}" >&2
    exit 1
  fi
}

require_cgroup_v2_for_node_image() {
  if [[ ! "${NODE_IMAGE}" =~ ^kindest/node:v1\.([0-9]+)\. ]]; then
    return
  fi

  local minor cgroup_version
  minor="${BASH_REMATCH[1]}"
  if ((minor < 35)); then
    return
  fi

  cgroup_version="$(docker info --format '{{.CgroupVersion}}' 2>/dev/null || true)"
  if [[ "${cgroup_version}" == "1" ]]; then
    echo "${NODE_IMAGE} requires cgroup v2; Docker reports cgroup v1" >&2
    exit 1
  fi
}

require_kind_for_node_image
require_cgroup_v2_for_node_image

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

# --- etcd provider smoke ---
# Prove the etcd provider restores a real snapshot under the Kubernetes runtime.
# etcd ships static binaries, so bake them onto Alpine to get a shell and tar for
# staging. The fixed non-:latest tag is used with the default IfNotPresent pull
# policy once loaded into the kind node.
ETCD_VERSION="${ETCD_VERSION:-v3.5.16}"
etcd_image="restore-drill-smoke-etcd:local"
etcd_build_dir="${tmp_dir}/etcd-image"
mkdir -p "${etcd_build_dir}"
cat > "${etcd_build_dir}/Dockerfile" <<DOCKER
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tar
RUN wget -qO /tmp/etcd.tar.gz https://github.com/etcd-io/etcd/releases/download/${ETCD_VERSION}/etcd-${ETCD_VERSION}-linux-amd64.tar.gz \\
 && tar xzf /tmp/etcd.tar.gz -C /tmp \\
 && mv /tmp/etcd-${ETCD_VERSION}-linux-amd64/etcd /tmp/etcd-${ETCD_VERSION}-linux-amd64/etcdctl /usr/local/bin/ \\
 && rm -rf /tmp/*
DOCKER
docker build -t "${etcd_image}" "${etcd_build_dir}" >/dev/null
kind load docker-image "${etcd_image}" --name "${CLUSTER_NAME}" >/dev/null

# Generate a real snapshot fixture; snapshot restore rejects empty files.
docker run --rm -v "${tmp_dir}:/out" "${etcd_image}" sh -ec '
export ETCDCTL_API=3
etcd --data-dir /tmp/seed \
  --listen-client-urls http://127.0.0.1:2379 \
  --advertise-client-urls http://127.0.0.1:2379 \
  --listen-peer-urls http://127.0.0.1:2380 \
  --initial-advertise-peer-urls http://127.0.0.1:2380 \
  --initial-cluster default=http://127.0.0.1:2380 \
  --name default > /tmp/etcd.log 2>&1 &
until etcdctl --endpoints=127.0.0.1:2379 endpoint health >/dev/null 2>&1; do sleep 0.2; done
etcdctl --endpoints=127.0.0.1:2379 put /registry/namespaces/default v1.Namespace >/dev/null
etcdctl --endpoints=127.0.0.1:2379 put /app/config/flag on >/dev/null
etcdctl --endpoints=127.0.0.1:2379 snapshot save /out/snapshot.db
chmod 0644 /out/snapshot.db
'

cat > "${tmp_dir}/etcd-drill.yaml" <<EOF
drills:
  - name: k8s-etcd-snapshot
    provider: etcd
    backup:
      tool: snapshot
      source: ${tmp_dir}/snapshot.db
    restore:
      timeout: 4m
      container:
        image: ${etcd_image}
    checks:
      - name: namespaces-present
        type: key_count
        key: /registry/namespaces/
        expect: "> 0"
      - name: default-namespace
        type: key_get
        key: /registry/namespaces/default
        expect: 'contains "Namespace"'
      - name: cluster-healthy
        type: query
        sql: "endpoint health"
        expect: 'contains "is healthy"'
EOF

go run ./cmd/restore-drill run \
  --runtime kubernetes \
  --kube-namespace "${NAMESPACE}" \
  --config "${tmp_dir}/etcd-drill.yaml" \
  --format json

kubectl get pods -n "${NAMESPACE}" -l restore-drill/ephemeral=true --no-headers 2>/dev/null | grep -v Terminating && {
  echo "restore-drill etcd ephemeral pods remain after smoke test" >&2
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
