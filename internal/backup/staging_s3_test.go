package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/internal/config"
	"github.com/RamazanKara/restore-drill/internal/engine"
)

func TestStageS3IsolatesConcurrentDownloads(t *testing.T) {
	for _, failCopy := range []bool{false, true} {
		t.Run(fmt.Sprintf("copy failure=%t", failCopy), func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
				t.Setenv(name, dir)
			}
			t.Setenv("AWS_ACCESS_KEY_ID", "restore-drill")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "restore-drill")
			t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
			const filename = "latest.sql.gz"
			sentinel := filepath.Join(dir, filename)
			if err := os.WriteFile(sentinel, []byte("unrelated file"), 0o600); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/backups/first/" + filename:
					_, _ = io.WriteString(w, "first backup")
				case "/backups/second/" + filename:
					_, _ = io.WriteString(w, "second backup")
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			cfg := config.BackupConfig{Repo: config.RepoConfig{
				Type: "s3", Bucket: "backups", Endpoint: server.URL, Prefix: "first/" + filename,
			}}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			rt := &blockingCopyRuntime{started: make(chan struct{}), release: make(chan struct{}, 1)}
			defer close(rt.release)
			firstDone := make(chan error, 1)
			go func() {
				_, err := Stage(ctx, rt, fakeContainer{}, cfg)
				firstDone <- err
			}()
			select {
			case <-rt.started:
			case err := <-firstDone:
				t.Fatalf("first staging stopped before copy: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			secondCfg := cfg
			secondCfg.Repo.Prefix = "second/" + filename
			secondRT := &fakeRuntime{}
			if failCopy {
				secondRT.copyErr = errors.New("copy failed")
			}
			staged, err := Stage(ctx, secondRT, fakeContainer{}, secondCfg)
			if failCopy {
				if !errors.Is(err, secondRT.copyErr) {
					t.Fatalf("expected copy failure, got %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if staged.Path != stageDir+"/"+filename || secondRT.copiedFiles[filename] != "second backup" {
					t.Fatalf("second backup was not preserved: %+v, %v", staged, secondRT.copiedFiles)
				}
			}
			rt.release <- struct{}{}
			if err := <-firstDone; err != nil {
				t.Fatalf("first staging: %v", err)
			}
			if rt.copiedFiles[filename] != "first backup" {
				t.Fatalf("first backup was overwritten: %v", rt.copiedFiles)
			}
			body, err := os.ReadFile(sentinel)
			if err != nil || string(body) != "unrelated file" {
				t.Fatalf("unrelated temporary file changed: %q, %v", body, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("download left temporary files behind: %v, %v", entries, err)
			}
		})
	}
}

type blockingCopyRuntime struct {
	fakeRuntime
	started chan struct{}
	release chan struct{}
}

func (r *blockingCopyRuntime) CopyTo(ctx context.Context, c engine.Container, dest string, src io.Reader) error {
	close(r.started)
	select {
	case <-r.release:
		return r.fakeRuntime.CopyTo(ctx, c, dest, src)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestStageS3DownloadsLatestObjectByPrefix(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "restore-drill")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "restore-drill")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	const (
		bucket     = "restore-drill-backups"
		oldKey     = "postgres/old.sql"
		latestKey  = "postgres/latest.sql"
		latestBody = "select 'latest';"
	)

	var listed bool
	var downloaded string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/"+bucket && r.URL.Query().Get("list-type") == "2" {
			listed = true
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>%s</Name>
  <Prefix>postgres/</Prefix>
  <KeyCount>2</KeyCount>
  <Contents>
    <Key>%s</Key>
    <LastModified>2026-05-20T10:00:00Z</LastModified>
    <ETag>"old"</ETag>
    <Size>1</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
  <Contents>
    <Key>%s</Key>
    <LastModified>2026-05-21T10:00:00Z</LastModified>
    <ETag>"latest"</ETag>
    <Size>%d</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
</ListBucketResult>`, bucket, oldKey, latestKey, len(latestBody))
			return
		}
		if r.URL.Path == "/"+bucket+"/"+latestKey {
			downloaded = latestKey
			_, _ = fmt.Fprint(w, latestBody)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	rt := &fakeRuntime{}
	staged, err := Stage(context.Background(), rt, fakeContainer{}, config.BackupConfig{
		Repo: config.RepoConfig{
			Type:     "s3",
			Bucket:   bucket,
			Endpoint: server.URL,
			Prefix:   "postgres/",
			Region:   "us-east-1",
		},
	})
	if err != nil {
		t.Fatalf("stage s3: %v", err)
	}

	if !listed {
		t.Fatal("expected S3 prefix listing request")
	}
	if downloaded != latestKey {
		t.Fatalf("expected latest key %q to be downloaded, got %q", latestKey, downloaded)
	}
	if staged.Path != "/tmp/restore-drill-backups/latest.sql" {
		t.Fatalf("unexpected staged path %q", staged.Path)
	}
	if staged.Description != "s3://restore-drill-backups/postgres/latest.sql" {
		t.Fatalf("unexpected staged description %q", staged.Description)
	}
	if got := rt.copiedFiles["latest.sql"]; got != latestBody {
		t.Fatalf("expected staged file body %q, got %q", latestBody, got)
	}
}

func TestStageS3EmptyPrefixReturnsActionableError(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "restore-drill")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "restore-drill")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	const bucket = "restore-drill-backups"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/"+bucket && r.URL.Query().Get("list-type") == "2" {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>%s</Name>
  <Prefix>postgres/</Prefix>
  <KeyCount>0</KeyCount>
</ListBucketResult>`, bucket)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := Stage(context.Background(), &fakeRuntime{}, fakeContainer{}, config.BackupConfig{
		Repo: config.RepoConfig{
			Type:     "s3",
			Bucket:   bucket,
			Endpoint: server.URL,
			Prefix:   "postgres/",
			Region:   "us-east-1",
		},
	})
	if err == nil {
		t.Fatal("expected empty prefix staging failure")
	}
	if !strings.Contains(err.Error(), "no objects found at s3://restore-drill-backups/postgres/") {
		t.Fatalf("expected empty prefix error, got %v", err)
	}
}
