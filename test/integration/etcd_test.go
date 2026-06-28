package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/pkg/engine"
	"github.com/RamazanKara/restore-drill/pkg/providers/etcd"
	"github.com/RamazanKara/restore-drill/pkg/reporter"
	"github.com/RamazanKara/restore-drill/pkg/runtime/docker"
)

// etcdVersion pins the etcd release whose static binaries are baked into the
// restore image and used to generate the snapshot fixture.
const etcdVersion = "v3.5.16"

func TestEtcdIntegration(t *testing.T) {
	if os.Getenv("RESTORE_DRILL_INTEGRATION") == "" {
		t.Skip("set RESTORE_DRILL_INTEGRATION=1 to run integration tests")
	}

	dir := t.TempDir()
	snapshot := filepath.Join(dir, "snapshot.db")
	cfgPath := filepath.Join(dir, "drill.yaml")

	// etcd ships static Go binaries; bake them onto Alpine so the restore image
	// also has a shell and tar for backup staging.
	image := buildDockerImage(t, "restore-drill-it-etcd", `
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tar
RUN wget -qO /tmp/etcd.tar.gz https://github.com/etcd-io/etcd/releases/download/`+etcdVersion+`/etcd-`+etcdVersion+`-linux-amd64.tar.gz \
 && tar xzf /tmp/etcd.tar.gz -C /tmp \
 && mv /tmp/etcd-`+etcdVersion+`-linux-amd64/etcd /tmp/etcd-`+etcdVersion+`-linux-amd64/etcdctl /usr/local/bin/ \
 && rm -rf /tmp/*
`)

	writeEtcdSnapshot(t, dir, image)

	writeFile(t, cfgPath, `
drills:
  - name: test-etcd-snapshot
    provider: etcd
    backup:
      tool: snapshot
      source: `+snapshot+`
    restore:
      timeout: 3m
      container:
        image: `+image+`
    checks:
      - name: namespace-count
        type: key_count
        key: /registry/namespaces/
        expect: ">= 2"
      - name: default-namespace-present
        type: key_get
        key: /registry/namespaces/default
        expect: 'contains "Namespace"'
      - name: total-keys
        type: key_count
        expect: ">= 3"
      - name: endpoint-healthy
        type: query
        sql: "endpoint health"
        expect: 'contains "is healthy"'
`)

	cfg, err := engine.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	rt, err := docker.New()
	if err != nil {
		t.Fatalf("init docker: %v", err)
	}

	eng := engine.New(rt, reporter.NewStdout())
	eng.RegisterProvider(etcd.New())

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	results, err := eng.Run(ctx, cfg.Drills)
	if err != nil {
		t.Fatalf("engine run: %v", err)
	}

	for _, result := range results {
		t.Run(result.Name, func(t *testing.T) {
			if result.Error != nil {
				t.Fatalf("drill error: %v", result.Error)
			}
			if !result.ValidationPassed {
				t.Fatalf("validation failed: %#v", result.Checks)
			}
		})
	}
}

// writeEtcdSnapshot starts etcd inside the image, writes a few keys, and saves a
// snapshot into the mounted host directory for use as a restore fixture.
func writeEtcdSnapshot(t *testing.T, dir, image string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	script := `
set -eu
export ETCDCTL_API=3
etcd --data-dir /tmp/seed \
  --listen-client-urls http://127.0.0.1:2379 \
  --advertise-client-urls http://127.0.0.1:2379 \
  --listen-peer-urls http://127.0.0.1:2380 \
  --initial-advertise-peer-urls http://127.0.0.1:2380 \
  --initial-cluster default=http://127.0.0.1:2380 \
  --name default > /tmp/etcd.log 2>&1 &
until etcdctl --endpoints=127.0.0.1:2379 endpoint health >/dev/null 2>&1; do sleep 0.2; done
etcdctl --endpoints=127.0.0.1:2379 put /registry/namespaces/default 'v1.Namespace:default'
etcdctl --endpoints=127.0.0.1:2379 put /registry/namespaces/kube-system 'v1.Namespace:kube-system'
etcdctl --endpoints=127.0.0.1:2379 put /app/config/feature-flags 'restore-drill=on'
etcdctl --endpoints=127.0.0.1:2379 snapshot save /out/snapshot.db
chmod 0644 /out/snapshot.db
`
	// #nosec G204 -- integration fixture generation invokes docker with a fixed image and temporary host directory.
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "-v", dir+":/out", image, "sh", "-ec", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate etcd snapshot fixture: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "snapshot.db")); err != nil {
		t.Fatalf("etcd snapshot fixture was not created: %v", err)
	}
}
