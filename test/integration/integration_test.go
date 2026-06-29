package integration

import (
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/internal/config"
	"github.com/RamazanKara/restore-drill/internal/engine"
	"github.com/RamazanKara/restore-drill/internal/providers/mysql"
	"github.com/RamazanKara/restore-drill/internal/providers/postgres"
	"github.com/RamazanKara/restore-drill/internal/providers/redis"
	"github.com/RamazanKara/restore-drill/internal/reporter"
	"github.com/RamazanKara/restore-drill/internal/runtime/docker"
)

func TestIntegration(t *testing.T) {
	if os.Getenv("RESTORE_DRILL_INTEGRATION") == "" {
		t.Skip("set RESTORE_DRILL_INTEGRATION=1 to run integration tests")
	}

	dir := t.TempDir()
	pgDump := filepath.Join(dir, "postgres.sql.gz")
	pgBackRestRepo := filepath.Join(dir, "pgbackrest")
	walGRepo := filepath.Join(dir, "walg")
	myDump := filepath.Join(dir, "mysql.sql.gz")
	xtraBackup := filepath.Join(dir, "xtrabackup")
	mariaBackup := filepath.Join(dir, "mariabackup")
	redisAOF := filepath.Join(dir, "appendonly.aof")
	redisRDB := filepath.Join(dir, "dump.rdb")
	cfgPath := filepath.Join(dir, "drill.yaml")
	pgBackRestImage := buildDockerImage(t, "restore-drill-it-pgbackrest", `
FROM postgres:16-alpine
USER root
RUN apk add --no-cache pgbackrest
USER postgres
`)
	xtraBackupImage := buildDockerImage(t, "restore-drill-it-xtrabackup", `
FROM percona/percona-xtrabackup:8.0.35
USER root
RUN microdnf install -y percona-server-server-8.0.35-27.1.el9 && microdnf clean all
ENTRYPOINT []
`)
	walGImage := buildDockerImage(t, "restore-drill-it-walg", `
FROM postgres:16-bookworm
USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl \
 && curl -fsSL -o /tmp/wal-g.tar.gz https://github.com/wal-g/wal-g/releases/download/v3.0.7/wal-g-pg-ubuntu-20.04-amd64.tar.gz \
 && echo "a573efb1d874ecfdd773a62cd718eb39adc1b065d27030046ecde9d252b291a4  /tmp/wal-g.tar.gz" | sha256sum -c - \
 && tar -xzf /tmp/wal-g.tar.gz -C /usr/local/bin \
 && mv /usr/local/bin/wal-g-pg-ubuntu-20.04-amd64 /usr/local/bin/wal-g \
 && chmod +x /usr/local/bin/wal-g \
 && rm -rf /var/lib/apt/lists/* /tmp/wal-g.tar.gz
USER postgres
`)

	writeGzipFile(t, pgDump, `
CREATE TABLE users (id integer primary key, name text);
INSERT INTO users (id, name) VALUES (1, 'ada'), (2, 'grace');
CREATE TABLE orders (id integer primary key, updated_at timestamptz);
INSERT INTO orders (id, updated_at) VALUES (1, now() - interval '5 minutes');
`)
	writePgBackRestFixture(t, dir, pgBackRestRepo, pgBackRestImage)
	writeWalGFixture(t, dir, walGRepo, walGImage)
	writeGzipFile(t, myDump, `
CREATE DATABASE app;
USE app;
CREATE TABLE users (id INT PRIMARY KEY, name VARCHAR(64));
INSERT INTO users (id, name) VALUES (1, 'ada'), (2, 'grace');
CREATE TABLE orders (id INT PRIMARY KEY, updated_at TIMESTAMP);
INSERT INTO orders (id, updated_at) VALUES (1, UTC_TIMESTAMP() - INTERVAL 5 MINUTE);
`)
	writeRedisAOF(t, redisAOF, [][]string{
		{"SELECT", "0"},
		{"SET", "session:health-check", "ok"},
		{"SET", "cache:hot", "warm"},
	})
	writeRedisRDBFixture(t, dir, redisRDB)
	writeXtraBackupFixture(t, dir, xtraBackup, xtraBackupImage)
	writeMariaBackupFixture(t, dir, mariaBackup)
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
      - name: orders-are-fresh
        type: freshness
        sql: "SELECT to_char((max(updated_at) AT TIME ZONE 'UTC'), 'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"') FROM orders"
        expect: "age < 1h"
      - name: default-extension-present
        type: extensions
        expect: "plpgsql"

  - name: test-postgres-pgbackrest
    provider: postgres
    backup:
      tool: pgbackrest
      stanza: main
      source: `+pgBackRestRepo+`
    restore:
      timeout: 4m
      container:
        image: `+pgBackRestImage+`
    checks:
      - name: users-exist
        type: sql
        sql: "SELECT count(*) FROM users"
        expect: "== 2"
      - name: users-table-exists
        type: schema
        sql: "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='users'"
        expect: "== 1"

  - name: test-postgres-walg
    provider: postgres
    backup:
      tool: wal-g
      source: `+walGRepo+`
    restore:
      timeout: 4m
      container:
        image: `+walGImage+`
    checks:
      - name: users-exist
        type: sql
        sql: "SELECT count(*) FROM users"
        expect: "== 2"
      - name: users-table-exists
        type: schema
        sql: "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='users'"
        expect: "== 1"

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
      - name: users-table-exists
        type: schema
        sql: "SELECT count(*) FROM information_schema.tables WHERE table_schema='app' AND table_name='users'"
        expect: "== 1"
      - name: orders-are-fresh
        type: freshness
        sql: "SELECT DATE_FORMAT(MAX(updated_at), '%Y-%m-%dT%H:%i:%sZ') FROM app.orders"
        expect: "age < 1h"

  - name: test-xtrabackup
    provider: mysql
    backup:
      tool: xtrabackup
      source: `+xtraBackup+`
    restore:
      timeout: 4m
      container:
        image: `+xtraBackupImage+`
    checks:
      - name: users-exist
        type: query
        sql: "SELECT count(*) FROM app.users"
        expect: "== 2"
      - name: users-table-exists
        type: schema
        sql: "SELECT count(*) FROM information_schema.tables WHERE table_schema='app' AND table_name='users'"
        expect: "== 1"

  - name: test-mariabackup
    provider: mysql
    backup:
      tool: mariabackup
      source: `+mariaBackup+`
    restore:
      timeout: 4m
      container:
        image: mariadb:11.4
        env:
          MARIADB_ALLOW_EMPTY_ROOT_PASSWORD: "yes"
    checks:
      - name: users-exist
        type: query
        sql: "SELECT count(*) FROM app.users"
        expect: "== 2"
      - name: users-table-exists
        type: schema
        sql: "SELECT count(*) FROM information_schema.tables WHERE table_schema='app' AND table_name='users'"
        expect: "== 1"

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
      - name: key-count
        type: key_count
        expect: "== 2"
      - name: session-key
        type: key_sample
        keys: ["session:health-check"]
        expect: "true"
      - name: session-value
        type: query
        sql: "GET session:health-check"
        expect: "ok"
      - name: ping
        type: query
        sql: "PING"
        expect: 'contains "PONG"'

  - name: test-redis-rdb
    provider: redis
    backup:
      tool: rdb
      source: `+redisRDB+`
    restore:
      timeout: 1m
      container:
        image: redis:7-alpine
    checks:
      - name: key-count
        type: key_count
        expect: "== 2"
      - name: session-key
        type: key_sample
        keys: ["session:health-check"]
        expect: "true"
      - name: cache-key-glob
        type: key_sample
        keys: ["cache:*"]
        expect: "true"
      - name: session-value
        type: query
        sql: "GET session:health-check"
        expect: "ok"
`)

	cfg, err := config.LoadConfig(cfgPath)
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

func writeGzipFile(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	gw := gzip.NewWriter(f)
	_, writeErr := gw.Write([]byte(content))
	closeGzipErr := gw.Close()
	closeFileErr := f.Close()
	if writeErr != nil {
		t.Fatalf("write gzip %s: %v", path, writeErr)
	}
	if closeGzipErr != nil {
		t.Fatalf("close gzip %s: %v", path, closeGzipErr)
	}
	if closeFileErr != nil {
		t.Fatalf("close %s: %v", path, closeFileErr)
	}
}

func writeRedisAOF(t *testing.T, path string, commands [][]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	for _, command := range commands {
		if _, err := fmt.Fprintf(f, "*%d\r\n", len(command)); err != nil {
			t.Fatalf("write AOF command length: %v", err)
		}
		for _, arg := range command {
			if _, err := fmt.Fprintf(f, "$%d\r\n%s\r\n", len(arg), arg); err != nil {
				t.Fatalf("write AOF argument: %v", err)
			}
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

func writeRedisRDBFixture(t *testing.T, dir, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	script := `
redis-server --daemonize yes --dir /data --dbfilename dump.rdb --appendonly no
until redis-cli PING >/dev/null 2>&1; do sleep 0.1; done
redis-cli SET session:health-check ok >/dev/null
redis-cli SET cache:hot warm >/dev/null
redis-cli SAVE >/dev/null
chmod 0644 /data/dump.rdb
redis-cli SHUTDOWN NOSAVE >/dev/null 2>&1 || true
`
	// #nosec G204 -- integration fixture generation invokes docker with a fixed image and temporary host directory.
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "-v", dir+":/data", "redis:7-alpine", "sh", "-ec", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate Redis RDB fixture: %v\n%s", err, out)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("redis RDB fixture was not created at %s: %v", path, err)
	}
}

func writePgBackRestFixture(t *testing.T, dir, repoPath, image string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	script := `
set -eu
trap 'chmod -R a+rwX /fixtures/pgdata /fixtures/pgbackrest 2>/dev/null || true' EXIT
chmod 0755 /fixtures
mkdir -p /fixtures/pgdata /fixtures/pgbackrest
chown -R postgres:postgres /fixtures/pgdata /fixtures/pgbackrest
chmod 0700 /fixtures/pgdata
su-exec postgres sh <<'PGSCRIPT'
set -eu
initdb -D /fixtures/pgdata --username=postgres --auth=trust >/tmp/initdb.log
cat >> /fixtures/pgdata/postgresql.conf <<'CONF'
archive_mode = on
archive_command = 'pgbackrest --stanza=main --repo1-path=/fixtures/pgbackrest --pg1-path=/fixtures/pgdata archive-push %p'
listen_addresses = '127.0.0.1'
CONF
pg_ctl -D /fixtures/pgdata -l /tmp/postgres.log -w start
psql -U postgres -d postgres <<'SQL'
CREATE TABLE users (id integer primary key, name text);
INSERT INTO users (id, name) VALUES (1, 'ada'), (2, 'grace');
SQL
pgbackrest --stanza=main --repo1-path=/fixtures/pgbackrest --pg1-path=/fixtures/pgdata stanza-create
pgbackrest --stanza=main --repo1-path=/fixtures/pgbackrest --pg1-path=/fixtures/pgdata --type=full backup
pg_ctl -D /fixtures/pgdata -m fast -w stop
PGSCRIPT
chmod -R a+rwX /fixtures/pgdata /fixtures/pgbackrest
`
	// #nosec G204 -- integration fixture generation invokes docker with a fixed image and temporary host directory.
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--user", "root", "-v", dir+":/fixtures", image, "sh", "-ec", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate pgBackRest fixture: %v\n%s", err, out)
	}
	if _, err := os.Stat(repoPath); err != nil {
		t.Fatalf("pgBackRest fixture was not created at %s: %v", repoPath, err)
	}
}

func writeWalGFixture(t *testing.T, dir, repoPath, image string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	script := `
set -eu
trap 'chmod -R a+rwX /fixtures/walgdata /fixtures/walg 2>/dev/null || true' EXIT
chmod 0755 /fixtures
mkdir -p /fixtures/walgdata /fixtures/walg
chown -R postgres:postgres /fixtures/walgdata /fixtures/walg
chmod 0700 /fixtures/walgdata
gosu postgres sh <<'PGSCRIPT'
set -eu
initdb -D /fixtures/walgdata --username=postgres --auth=trust >/tmp/walg-initdb.log
cat >> /fixtures/walgdata/postgresql.conf <<'CONF'
archive_mode = on
archive_command = 'WALG_FILE_PREFIX=/fixtures/walg wal-g wal-push %p'
listen_addresses = '127.0.0.1'
CONF
pg_ctl -D /fixtures/walgdata -l /tmp/walg-postgres.log -w start
psql -U postgres -d postgres <<'SQL'
CREATE TABLE users (id integer primary key, name text);
INSERT INTO users (id, name) VALUES (1, 'ada'), (2, 'grace');
SQL
PGHOST=/var/run/postgresql PGUSER=postgres WALG_FILE_PREFIX=/fixtures/walg wal-g backup-push /fixtures/walgdata
pg_ctl -D /fixtures/walgdata -m fast -w stop
PGSCRIPT
chmod -R a+rwX /fixtures/walgdata /fixtures/walg
`
	// #nosec G204 -- integration fixture generation invokes docker with a fixed image and temporary host directory.
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--user", "root", "-v", dir+":/fixtures", image, "sh", "-ec", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate WAL-G fixture: %v\n%s", err, out)
	}
	if _, err := os.Stat(repoPath); err != nil {
		t.Fatalf("WAL-G fixture was not created at %s: %v", repoPath, err)
	}
}

func writeXtraBackupFixture(t *testing.T, dir, path, image string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	script := `
set -eu
trap 'chmod -R a+rwX /fixtures/xtrabackup-source /fixtures/xtrabackup 2>/dev/null || true' EXIT
chmod 0755 /fixtures
mkdir -p /fixtures/xtrabackup-source /fixtures/xtrabackup
chown -R mysql:mysql /fixtures/xtrabackup-source /fixtures/xtrabackup
mysqld --initialize-insecure --datadir=/fixtures/xtrabackup-source --user=mysql >/tmp/xtrabackup-mysql-init.log 2>&1
mysqld_safe --datadir=/fixtures/xtrabackup-source --socket=/tmp/xtrabackup-mysql.sock --port=3307 --skip-networking=0 --bind-address=127.0.0.1 --user=mysql >/tmp/xtrabackup-mysql.log 2>&1 &
until mysqladmin -u root --socket=/tmp/xtrabackup-mysql.sock ping >/dev/null 2>&1; do sleep 0.2; done
mysql -u root --socket=/tmp/xtrabackup-mysql.sock <<'SQL'
CREATE DATABASE app;
CREATE TABLE app.users (id INT PRIMARY KEY, name VARCHAR(64));
INSERT INTO app.users (id, name) VALUES (1, 'ada'), (2, 'grace');
SQL
xtrabackup --backup --target-dir=/fixtures/xtrabackup --datadir=/fixtures/xtrabackup-source --user=root --socket=/tmp/xtrabackup-mysql.sock >/tmp/xtrabackup.log 2>&1
mysqladmin -u root --socket=/tmp/xtrabackup-mysql.sock shutdown
chmod -R a+rwX /fixtures/xtrabackup-source /fixtures/xtrabackup
`
	// #nosec G204 -- integration fixture generation invokes docker with a fixed image and temporary host directory.
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--user", "root", "-v", dir+":/fixtures", image, "sh", "-ec", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate xtrabackup fixture: %v\n%s", err, out)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("xtrabackup fixture was not created at %s: %v", path, err)
	}
}

func writeMariaBackupFixture(t *testing.T, dir, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	script := `
set -eu
mkdir -p /fixtures/mariadb-source /fixtures/mariabackup
chmod 0755 /fixtures
chown -R mysql:mysql /fixtures/mariadb-source /fixtures/mariabackup
mariadb-install-db --datadir=/fixtures/mariadb-source --auth-root-authentication-method=normal --user=mysql --skip-test-db >/tmp/mariadb-install.log
mariadbd-safe --datadir=/fixtures/mariadb-source --socket=/tmp/mariadb.sock --port=3306 --skip-networking=0 --bind-address=127.0.0.1 --user=mysql >/tmp/mariadb.log 2>&1 &
until mariadb-admin -u root --socket=/tmp/mariadb.sock ping >/dev/null 2>&1; do sleep 0.2; done
mariadb -u root --socket=/tmp/mariadb.sock <<'SQL'
CREATE DATABASE app;
CREATE TABLE app.users (id INT PRIMARY KEY, name VARCHAR(64));
INSERT INTO app.users (id, name) VALUES (1, 'ada'), (2, 'grace');
SQL
mariabackup --backup --target-dir=/fixtures/mariabackup --datadir=/fixtures/mariadb-source --socket=/tmp/mariadb.sock --user=root >/tmp/mariabackup.log
mariadb-admin -u root --socket=/tmp/mariadb.sock shutdown >/dev/null 2>&1 || true
chmod -R a+rwX /fixtures/mariadb-source /fixtures/mariabackup
`
	// #nosec G204 -- integration fixture generation invokes docker with a fixed image and temporary host directory.
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "-v", dir+":/fixtures", "mariadb:11.4", "sh", "-ec", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate MariaDB physical fixture: %v\n%s", err, out)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("mariabackup fixture was not created at %s: %v", path, err)
	}
}

func buildDockerImage(t *testing.T, name, dockerfile string) string {
	t.Helper()
	tag := fmt.Sprintf("%s:%d", name, time.Now().UnixNano())
	ctxDir := t.TempDir()
	writeFile(t, filepath.Join(ctxDir, "Dockerfile"), dockerfile)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "build", "-t", tag, ctxDir) // #nosec G204 -- integration tests build a fixed temporary Dockerfile created by the test.
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build integration image %s: %v\n%s", tag, err, out)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", "image", "rm", "-f", tag) // #nosec G204 -- tag is generated by this test helper.
		_ = cmd.Run()
	})
	return tag
}
