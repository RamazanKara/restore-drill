package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

type fakeContainer struct{}

func (fakeContainer) ID() string             { return "fake" }
func (fakeContainer) Host() string           { return "127.0.0.1" }
func (fakeContainer) Port(container int) int { return container }
func (fakeContainer) String() string         { return "fake" }
func (fakeContainer) GoString() string       { return "fake" }

type fakeRuntime struct {
	copiedNames []string
}

func (f *fakeRuntime) Create(ctx context.Context, spec engine.ContainerSpec) (engine.Container, error) {
	return fakeContainer{}, nil
}

func (f *fakeRuntime) Exec(ctx context.Context, c engine.Container, cmd []string) ([]byte, error) {
	return nil, nil
}

func (f *fakeRuntime) CopyTo(ctx context.Context, c engine.Container, dest string, src io.Reader) error {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, src); err != nil {
		return err
	}
	tr := tar.NewReader(&buf)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		f.copiedNames = append(f.copiedNames, hdr.Name)
	}
}

func (f *fakeRuntime) Destroy(ctx context.Context, c engine.Container) error {
	return nil
}

func (f *fakeRuntime) Logs(ctx context.Context, c engine.Container) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func TestStageLocalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dump.sql")
	if err := os.WriteFile(path, []byte("select 1;"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	rt := &fakeRuntime{}
	staged, err := Stage(context.Background(), rt, fakeContainer{}, engine.BackupConfig{Source: path})
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if staged.Path != "/tmp/restore-drill-backups/dump.sql" {
		t.Fatalf("unexpected staged path %q", staged.Path)
	}
	if len(rt.copiedNames) != 1 || rt.copiedNames[0] != "dump.sql" {
		t.Fatalf("unexpected copied tar entries: %#v", rt.copiedNames)
	}
}

func TestStageTargetPathWhenHostFileDoesNotExist(t *testing.T) {
	rt := &fakeRuntime{}
	staged, err := Stage(context.Background(), rt, fakeContainer{}, engine.BackupConfig{Source: "/mounted/dump.sql"})
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if staged.Path != "/mounted/dump.sql" {
		t.Fatalf("unexpected staged path %q", staged.Path)
	}
	if len(rt.copiedNames) != 0 {
		t.Fatalf("target path should not be copied: %#v", rt.copiedNames)
	}
}
