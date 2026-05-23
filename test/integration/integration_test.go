package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/pkg/engine"
	"github.com/RamazanKara/restore-drill/pkg/providers/mysql"
	"github.com/RamazanKara/restore-drill/pkg/providers/postgres"
	"github.com/RamazanKara/restore-drill/pkg/providers/redis"
	"github.com/RamazanKara/restore-drill/pkg/reporter"
	"github.com/RamazanKara/restore-drill/pkg/runtime/docker"
)

func TestIntegration(t *testing.T) {
	if os.Getenv("RESTORE_DRILL_INTEGRATION") == "" {
		t.Skip("set RESTORE_DRILL_INTEGRATION=1 to run integration tests")
	}

	dir := t.TempDir()
	pgDump := filepath.Join(dir, "postgres.sql")
	myDump := filepath.Join(dir, "mysql.sql")
	redisAOF := filepath.Join(dir, "appendonly.aof")
	cfgPath := filepath.Join(dir, "drill.yaml")

	writeFile(t, pgDump, `
CREATE TABLE users (id integer primary key, name text);
INSERT INTO users (id, name) VALUES (1, 'ada'), (2, 'grace');
`)
	writeFile(t, myDump, `
CREATE DATABASE app;
USE app;
CREATE TABLE users (id INT PRIMARY KEY, name VARCHAR(64));
INSERT INTO users (id, name) VALUES (1, 'ada'), (2, 'grace');
`)
	writeFile(t, redisAOF, "")
	writeFile(t, cfgPath, `
drills:
  - name: test-postgres-dump
    provider: postgres
    backup:
      tool: pg_dump
      source: `+pgDump+`
    restore:
      timeout: 2m
      container:
        image: postgres:16-alpine
        env:
          POSTGRES_HOST_AUTH_METHOD: trust
    checks:
      - name: users-exist
        type: sql
        sql: "SELECT count(*) FROM users"
        expect: "== 2"

  - name: test-mysql-dump
    provider: mysql
    backup:
      tool: mysqldump
      source: `+myDump+`
    restore:
      timeout: 2m
      container:
        image: mysql:8
        env:
          MYSQL_ALLOW_EMPTY_PASSWORD: "yes"
    checks:
      - name: users-exist
        type: query
        sql: "SELECT count(*) FROM app.users"
        expect: "== 2"

  - name: test-redis-aof
    provider: redis
    backup:
      tool: aof
      source: `+redisAOF+`
    restore:
      timeout: 1m
      container:
        image: redis:7-alpine
    checks:
      - name: ping
        type: query
        sql: "PING"
        expect: 'contains "PONG"'
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
	eng.RegisterProvider(postgres.New())
	eng.RegisterProvider(mysql.New())
	eng.RegisterProvider(redis.New())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
