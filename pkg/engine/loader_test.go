package engine

import (
	"os"
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
	os.Setenv("TEST_BUCKET", "my-bucket")
	defer os.Unsetenv("TEST_BUCKET")

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
	os.Unsetenv("UNSET_VAR")

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
