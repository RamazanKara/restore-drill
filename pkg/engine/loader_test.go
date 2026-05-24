package engine

import (
	"os"
	"strings"
	"testing"
)

func TestParseConfig_Valid(t *testing.T) {
	yaml := `
drills:
  - name: test-pg
    provider: postgres
    backup:
      tool: pgbackrest
      stanza: main
      source: s3://bucket/path
    restore:
      container:
        image: postgres:16
    checks:
      - name: row_count
        type: query
        sql: "SELECT count(*) FROM users"
        expect: "> 0"
`
	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Drills) != 1 {
		t.Fatalf("expected 1 drill, got %d", len(cfg.Drills))
	}
	if cfg.Drills[0].Name != "test-pg" {
		t.Errorf("expected name 'test-pg', got %q", cfg.Drills[0].Name)
	}
	if cfg.Drills[0].Provider != "postgres" {
		t.Errorf("expected provider 'postgres', got %q", cfg.Drills[0].Provider)
	}
}

func TestParseConfig_EnvInterpolation(t *testing.T) {
	t.Setenv("TEST_BUCKET", "my-bucket")

	yaml := `
drills:
  - name: test-pg
    provider: postgres
    backup:
      tool: pg_dump
      source: "s3://${TEST_BUCKET}/backups"
    restore:
      container:
        image: postgres:16
    checks:
      - name: check
        type: query
        sql: "SELECT 1"
        expect: "== 1"
`
	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Drills[0].Backup.Source != "s3://my-bucket/backups" {
		t.Errorf("env interpolation failed, got %q", cfg.Drills[0].Backup.Source)
	}
}

func TestParseConfig_EnvDefault(t *testing.T) {
	original, hadOriginal := os.LookupEnv("UNSET_VAR")
	if err := os.Unsetenv("UNSET_VAR"); err != nil {
		t.Fatalf("unset env: %v", err)
	}
	t.Cleanup(func() {
		var err error
		if hadOriginal {
			err = os.Setenv("UNSET_VAR", original)
		} else {
			err = os.Unsetenv("UNSET_VAR")
		}
		if err != nil {
			t.Fatalf("restore env: %v", err)
		}
	})

	yaml := `
drills:
  - name: test-pg
    provider: postgres
    backup:
      tool: pg_dump
      source: "s3://${UNSET_VAR:-default-bucket}/backups"
    restore:
      container:
        image: postgres:16
    checks:
      - name: check
        type: query
        sql: "SELECT 1"
        expect: "== 1"
`
	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Drills[0].Backup.Source != "s3://default-bucket/backups" {
		t.Errorf("default interpolation failed, got %q", cfg.Drills[0].Backup.Source)
	}
}

func TestParseConfig_ProviderToolAliases(t *testing.T) {
	tests := []struct {
		name string
		tool string
	}{
		{name: "pg restore", tool: "pg_restore"},
		{name: "walg alias", tool: "walg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := `
drills:
  - name: test-pg
    provider: postgres
    backup:
      tool: ` + tt.tool + `
      source: /backups/latest
    restore:
      container:
        image: postgres:16
    checks:
      - name: check
        type: query
        sql: "SELECT 1"
        expect: "== 1"
`
			if _, err := ParseConfig([]byte(yaml)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseConfig_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "no drills",
			yaml: `drills: []`,
		},
		{
			name: "missing name",
			yaml: `
drills:
  - provider: postgres
    backup:
      tool: pgbackrest
    restore:
      container:
        image: postgres:16
`,
		},
		{
			name: "missing provider",
			yaml: `
drills:
  - name: test
    backup:
      tool: pgbackrest
    restore:
      container:
        image: postgres:16
`,
		},
		{
			name: "unknown provider",
			yaml: `
drills:
  - name: test
    provider: mongodb
    backup:
      tool: mongodump
    restore:
      container:
        image: mongo:7
`,
		},
		{
			name: "missing backup tool",
			yaml: `
drills:
  - name: test
    provider: postgres
    backup:
      source: s3://bucket
    restore:
      container:
        image: postgres:16
`,
		},
		{
			name: "missing image",
			yaml: `
drills:
  - name: test
    provider: postgres
    backup:
      tool: pgbackrest
    restore:
      container:
        image: ""
`,
		},
		{
			name: "duplicate drill name",
			yaml: `
drills:
  - name: test
    provider: postgres
    backup:
      tool: pgbackrest
    restore:
      container:
        image: postgres:16
  - name: test
    provider: mysql
    backup:
      tool: mysqldump
    restore:
      container:
        image: mysql:8
`,
		},
		{
			name: "invalid timeout",
			yaml: `
drills:
  - name: test
    provider: postgres
    backup:
      tool: pgbackrest
    restore:
      timeout: "invalid"
      container:
        image: postgres:16
`,
		},
		{
			name: "check missing expect",
			yaml: `
drills:
  - name: test
    provider: postgres
    backup:
      tool: pgbackrest
    restore:
      container:
        image: postgres:16
    checks:
      - name: check1
        type: query
        sql: "SELECT 1"
`,
		},
		{
			name: "unsupported backup tool for provider",
			yaml: `
drills:
  - name: test
    provider: redis
    backup:
      tool: pg_dump
      source: /backups/dump.rdb
    restore:
      container:
        image: redis:7-alpine
`,
		},
		{
			name: "unsupported repo type",
			yaml: `
drills:
  - name: test
    provider: postgres
    backup:
      tool: pg_dump
      repo:
        type: gcs
        bucket: backups
        prefix: postgres/latest.sql
    restore:
      container:
        image: postgres:16
`,
		},
		{
			name: "repo missing bucket",
			yaml: `
drills:
  - name: test
    provider: mysql
    backup:
      tool: mysqldump
      repo:
        type: s3
        prefix: mysql/latest.sql
    restore:
      container:
        image: mysql:8
`,
		},
		{
			name: "freshness check missing sql",
			yaml: `
drills:
  - name: test
    provider: postgres
    backup:
      tool: pg_dump
      source: /backups/latest.sql
    restore:
      container:
        image: postgres:16
    checks:
      - name: freshness
        type: freshness
        expect: "age < 24h"
`,
		},
		{
			name: "key sample missing keys",
			yaml: `
drills:
  - name: test
    provider: redis
    backup:
      tool: aof
      source: /backups/appendonly.aof
    restore:
      container:
        image: redis:7-alpine
    checks:
      - name: sample
        type: key_sample
        expect: exists
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tt.yaml))
			if err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

func TestParseConfig_ProviderSpecificCheckValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "redis rejects schema",
			yaml: `
drills:
  - name: redis
    provider: redis
    backup:
      tool: aof
      source: /backups/appendonly.aof
    restore:
      container:
        image: redis:7-alpine
    checks:
      - name: schema
        type: schema
        sql: "SELECT 1"
        expect: exists
`,
			wantErr: `unsupported type "schema" for provider "redis"`,
		},
		{
			name: "mysql rejects extensions",
			yaml: `
drills:
  - name: mysql
    provider: mysql
    backup:
      tool: mysqldump
      source: /backups/latest.sql
    restore:
      container:
        image: mysql:8
    checks:
      - name: extensions
        type: extensions
        expect: pgcrypto
`,
			wantErr: `unsupported type "extensions" for provider "mysql"`,
		},
		{
			name: "postgres rejects key sample",
			yaml: `
drills:
  - name: postgres
    provider: postgres
    backup:
      tool: pg_dump
      source: /backups/latest.sql
    restore:
      container:
        image: postgres:16
    checks:
      - name: keys
        type: key_sample
        keys: ["user:*"]
        expect: exists
`,
			wantErr: `unsupported type "key_sample" for provider "postgres"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tt.yaml))
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestDrillTimeout_Default(t *testing.T) {
	d := &DrillConfig{}
	timeout := d.DrillTimeout()
	if timeout.Minutes() != 10 {
		t.Errorf("expected 10m default, got %v", timeout)
	}
}

func TestDrillTimeout_Custom(t *testing.T) {
	d := &DrillConfig{Restore: RestoreSpec{Timeout: "5m"}}
	timeout := d.DrillTimeout()
	if timeout.Minutes() != 5 {
		t.Errorf("expected 5m, got %v", timeout)
	}
}
