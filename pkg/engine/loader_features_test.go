package engine

import (
	"strings"
	"testing"
)

func TestParseConfigAcceptsEtcdAndSlack(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
drills:
  - name: etcd-prod
    provider: etcd
    backup:
      tool: snapshot
      source: /backups/etcd/snapshot.db
    restore:
      container:
        image: example/etcd:3.5
    checks:
      - name: keys
        type: key_count
        key: /registry/
        expect: "> 0"
      - name: default-namespace
        type: key_get
        key: /registry/namespaces/default
        expect: contains "default"
    alerts:
      - type: slack
        url: https://hooks.slack.com/services/x
        on: failure
`))
	if err != nil {
		t.Fatalf("expected valid etcd/slack config, got %v", err)
	}
	if got := cfg.Drills[0].Alerts[0].On; got != "failure" {
		t.Errorf("expected alert on=failure, got %q", got)
	}
	if got := cfg.Drills[0].Validate[1].Key; got != "/registry/namespaces/default" {
		t.Errorf("expected key_get key preserved, got %q", got)
	}
}

func TestParseConfigRejectsInvalidFeatureConfig(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "etcd unsupported tool",
			yaml:    baseEtcd(`tool: rdb`, "", `type: key_count`+"\n        expect: \"> 0\""),
			wantErr: `unsupported backup.tool "rdb"`,
		},
		{
			name:    "key_get requires key",
			yaml:    baseEtcd(`tool: snapshot`, "", `type: key_get`+"\n        expect: exists"),
			wantErr: `of type "key_get" must have key`,
		},
		{
			name:    "slack alert needs url",
			yaml:    baseEtcd(`tool: snapshot`, "\n    alerts:\n      - type: slack", `type: key_count`+"\n        expect: \"> 0\""),
			wantErr: `slack alert[0] must specify url or endpoint`,
		},
		{
			name:    "bad on condition",
			yaml:    baseEtcd(`tool: snapshot`, "\n    alerts:\n      - type: webhook\n        url: https://x\n        on: sometimes", `type: key_count`+"\n        expect: \"> 0\""),
			wantErr: `unsupported on "sometimes"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

// baseEtcd builds a minimal etcd drill config with the given backup line, an
// optional extra block (e.g. alerts), and a single check body.
func baseEtcd(backupLine, extra, checkBody string) string {
	return `
drills:
  - name: etcd-prod
    provider: etcd
    backup:
      ` + backupLine + `
      source: /backups/etcd/snapshot.db
    restore:
      container:
        image: example/etcd:3.5` + extra + `
    checks:
      - name: c1
        ` + checkBody + `
`
}
