package backup

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

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
	staged, err := Stage(context.Background(), rt, fakeContainer{}, engine.BackupConfig{
		Repo: engine.RepoConfig{
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
