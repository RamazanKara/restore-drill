package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RamazanKara/restore-drill/internal/config"
	"github.com/RamazanKara/restore-drill/internal/engine"
)

type fakeContainer struct{}

func (fakeContainer) ID() string             { return "fake" }
func (fakeContainer) Host() string           { return "127.0.0.1" }
func (fakeContainer) Port(container int) int { return container }
func (fakeContainer) String() string         { return "fake" }
func (fakeContainer) GoString() string       { return "fake" }

type fakeRuntime struct {
	copiedNames  []string
	copiedFiles  map[string]string
	execCommands [][]string
	execOutput   []byte
	execErr      error
	copyErr      error
}

func (f *fakeRuntime) Create(ctx context.Context, spec engine.ContainerSpec) (engine.Container, error) {
	return fakeContainer{}, nil
}

func (f *fakeRuntime) Exec(ctx context.Context, c engine.Container, cmd []string) ([]byte, error) {
	f.execCommands = append(f.execCommands, append([]string(nil), cmd...))
	if f.execErr != nil {
		return nil, f.execErr
	}
	return f.execOutput, nil
}

func (f *fakeRuntime) CopyTo(ctx context.Context, c engine.Container, dest string, src io.Reader) error {
	if f.copyErr != nil {
		return f.copyErr
	}
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
		if hdr.FileInfo().IsDir() {
			continue
		}
		var content bytes.Buffer
		if _, err := io.Copy(&content, io.LimitReader(tr, 1<<20)); err != nil {
			return err
		}
		if f.copiedFiles == nil {
			f.copiedFiles = make(map[string]string)
		}
		f.copiedFiles[hdr.Name] = content.String()
	}
}

func (f *fakeRuntime) Destroy(ctx context.Context, c engine.Container) error {
	return nil
}

func TestStageLocalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dump.sql")
	if err := os.WriteFile(path, []byte("select 1;"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	rt := &fakeRuntime{}
	staged, err := Stage(context.Background(), rt, fakeContainer{}, config.BackupConfig{Source: path})
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

func TestStageLocalCopyFailureIncludesTargetDiagnostics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dump.sql")
	if err := os.WriteFile(path, []byte("select 1;"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	rt := &fakeRuntime{
		copyErr:    errors.New("permission denied"),
		execOutput: []byte("uid=65534(nobody) gid=65534(nobody)"),
	}
	_, err := Stage(context.Background(), rt, fakeContainer{}, config.BackupConfig{Source: path})
	if err == nil {
		t.Fatal("expected staging copy failure")
	}
	if !strings.Contains(err.Error(), "copy backup source into target: permission denied") {
		t.Fatalf("expected copy error context, got %v", err)
	}
	if !strings.Contains(err.Error(), "target staging diagnostics: uid=65534") {
		t.Fatalf("expected target diagnostics, got %v", err)
	}
}

func TestWriteTarPreservesSymlinkTargets(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "20260524-restore")
	if err := os.WriteFile(target, []byte("backup"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(dir, "latest")
	if err := os.Symlink(filepath.Base(target), link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}

	var buf bytes.Buffer
	if err := writeTar(&buf, dir, info); err != nil {
		t.Fatalf("write tar: %v", err)
	}

	tr := tar.NewReader(&buf)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar: %v", err)
		}
		if hdr.Name == filepath.Base(dir)+"/latest" {
			if hdr.Typeflag != tar.TypeSymlink {
				t.Fatalf("expected symlink header, got %v", hdr.Typeflag)
			}
			if hdr.Linkname != filepath.Base(target) {
				t.Fatalf("expected link target %q, got %q", filepath.Base(target), hdr.Linkname)
			}
			return
		}
	}
	t.Fatal("symlink entry not found")
}

func TestStageTargetPathWhenHostFileDoesNotExist(t *testing.T) {
	rt := &fakeRuntime{}
	staged, err := Stage(context.Background(), rt, fakeContainer{}, config.BackupConfig{Source: "/mounted/dump.sql"})
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

func TestArchiveRequirements(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{path: "/backups/full.tar", want: []string{"tar"}},
		{path: "/backups/full.tgz", want: []string{"tar"}},
		{path: "/backups/full.tar.gz", want: []string{"tar"}},
		{path: "/backups/full.xbstream", want: []string{"xbstream"}},
		{path: "/backups/full.xbstream.gz", want: []string{"gzip", "xbstream"}},
		{path: "/backups/full", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := ArchiveRequirements(tt.path)
			if len(got) != len(tt.want) {
				t.Fatalf("expected %#v, got %#v", tt.want, got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("expected %#v, got %#v", tt.want, got)
				}
			}
		})
	}
}

func TestMaterializeArchiveExtractsIntoTarget(t *testing.T) {
	rt := &fakeRuntime{execOutput: []byte("/tmp/restore-drill-backups/mysql-physical/full")}

	got, err := MaterializeArchive(
		context.Background(),
		rt,
		fakeContainer{},
		"/tmp/restore-drill-backups/full.tar.gz",
		"/tmp/restore-drill-backups/mysql-physical",
	)
	if err != nil {
		t.Fatalf("materialize archive: %v", err)
	}
	if got != "/tmp/restore-drill-backups/mysql-physical/full" {
		t.Fatalf("unexpected materialized path %q", got)
	}
	if len(rt.execCommands) != 1 {
		t.Fatalf("expected one extraction command, got %#v", rt.execCommands)
	}
	cmd := rt.execCommands[0]
	if len(cmd) != 3 || cmd[0] != "sh" || cmd[1] != "-c" {
		t.Fatalf("unexpected extraction command %#v", cmd)
	}
	if !strings.Contains(cmd[2], "tar -xzf") {
		t.Fatalf("expected tar extraction command, got %q", cmd[2])
	}
}

func TestMaterializeArchiveLeavesDirectoryPathUnchanged(t *testing.T) {
	rt := &fakeRuntime{}
	got, err := MaterializeArchive(context.Background(), rt, fakeContainer{}, "/mounted/xtrabackup", "/tmp/out")
	if err != nil {
		t.Fatalf("materialize archive: %v", err)
	}
	if got != "/mounted/xtrabackup" {
		t.Fatalf("unexpected path %q", got)
	}
	if len(rt.execCommands) != 0 {
		t.Fatalf("non-archive path should not run extraction commands: %#v", rt.execCommands)
	}
}
