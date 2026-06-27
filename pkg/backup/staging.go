// Package backup stages backup artifacts into ephemeral restore targets.
package backup

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/RamazanKara/restore-drill/pkg/engine"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const stageDir = "/tmp/restore-drill-backups"

// StagedBackup describes a backup artifact that is available inside a target.
type StagedBackup struct {
	Path        string
	Description string
}

// Stage makes cfg available inside target and returns the in-target path.
//
// Local files and directories are copied into a staging directory. If Source
// does not exist on the host, it is treated as a path already mounted inside
// the target. S3-compatible repos are downloaded locally and then copied in.
func Stage(ctx context.Context, rt engine.Runtime, target engine.Container, cfg engine.BackupConfig) (*StagedBackup, error) {
	if cfg.Source != "" {
		staged, err := stageSource(ctx, rt, target, cfg.Source)
		if err != nil {
			return nil, err
		}
		return staged, nil
	}

	if cfg.Repo.Type != "" {
		switch strings.ToLower(cfg.Repo.Type) {
		case "s3", "s3-compatible":
			return stageS3(ctx, rt, target, cfg.Repo)
		default:
			return nil, fmt.Errorf("unsupported backup repo type %q", cfg.Repo.Type)
		}
	}

	return nil, errors.New("backup source or repo must be configured")
}

func stageSource(ctx context.Context, rt engine.Runtime, target engine.Container, source string) (*StagedBackup, error) {
	if strings.HasPrefix(source, "s3://") {
		repo, err := repoFromS3URI(source)
		if err != nil {
			return nil, err
		}
		return stageS3(ctx, rt, target, repo)
	}

	info, err := os.Stat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return &StagedBackup{Path: source, Description: "target:" + source}, nil
		}
		return nil, fmt.Errorf("stat backup source %q: %w", source, err)
	}

	if err := ensureStageDir(ctx, rt, target); err != nil {
		return nil, err
	}

	stagedPath := filepath.ToSlash(filepath.Join(stageDir, filepath.Base(source)))
	pr, pw := io.Pipe()
	tarErrCh := make(chan error, 1)
	go func() {
		err := writeTar(pw, source, info)
		tarErrCh <- err
		_ = pw.CloseWithError(err)
	}()

	copyErr := rt.CopyTo(ctx, target, stageDir+"/", pr)
	if copyErr != nil {
		_ = pr.Close()
		if tarErr := <-tarErrCh; tarErr != nil && !errors.Is(tarErr, io.ErrClosedPipe) {
			return nil, fmt.Errorf("archive backup source for copy: %w", tarErr)
		}
		if out, diagErr := rt.Exec(ctx, target, []string{"sh", "-c", "id; ls -ld /tmp /tmp/restore-drill-backups 2>&1"}); diagErr == nil {
			return nil, fmt.Errorf("copy backup source into target: %w (target staging diagnostics: %s)", copyErr, strings.TrimSpace(string(out)))
		}
		return nil, fmt.Errorf("copy backup source into target: %w", copyErr)
	}
	if tarErr := <-tarErrCh; tarErr != nil {
		return nil, fmt.Errorf("archive backup source for copy: %w", tarErr)
	}

	slog.Info("staged backup source", "source", source, "target", stagedPath)
	return &StagedBackup{Path: stagedPath, Description: "local:" + source}, nil
}

func repoFromS3URI(uri string) (engine.RepoConfig, error) {
	trimmed := strings.TrimPrefix(uri, "s3://")
	bucket, key, ok := strings.Cut(trimmed, "/")
	if !ok || bucket == "" || key == "" {
		return engine.RepoConfig{}, fmt.Errorf("invalid s3 source %q (use s3://bucket/key-or-prefix)", uri)
	}
	return engine.RepoConfig{Type: "s3", Bucket: bucket, Prefix: key}, nil
}

func ensureStageDir(ctx context.Context, rt engine.Runtime, target engine.Container) error {
	script := "command -v tar >/dev/null 2>&1 || { echo 'restore-drill staging requires tar in the restore target image' >&2; exit 127; }; mkdir -p " + stageDir + " && chmod 0777 " + stageDir
	if _, err := rt.Exec(ctx, target, []string{"sh", "-c", script}); err != nil {
		return fmt.Errorf("create target staging directory: %w", err)
	}
	return nil
}

func writeTar(w io.Writer, source string, info os.FileInfo) error {
	tw := tar.NewWriter(w)
	var err error
	base := filepath.Base(source)
	if !info.IsDir() {
		err = addFileToTar(tw, source, base, info)
	} else {
		err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			entryInfo, err := entry.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			name := filepath.ToSlash(filepath.Join(base, rel))
			if rel == "." {
				name = base
			}
			return addFileToTar(tw, path, name, entryInfo)
		})
	}
	if closeErr := tw.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	return err
}

func addFileToTar(tw *tar.Writer, path, name string, info os.FileInfo) error {
	linkTarget := ""
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return fmt.Errorf("read symlink %s: %w", path, err)
		}
		linkTarget = target
	}

	hdr, err := tar.FileInfoHeader(info, linkTarget)
	if err != nil {
		return fmt.Errorf("tar header for %s: %w", path, err)
	}
	hdr.Name = name
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write tar header for %s: %w", path, err)
	}
	if info.IsDir() {
		return nil
	}
	if !info.Mode().IsRegular() {
		return nil
	}

	f, err := os.Open(path) // #nosec G304 -- path comes from the explicit backup source being staged.
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	_, copyErr := io.Copy(tw, f)
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("copy %s into tar: %w", path, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", path, closeErr)
	}
	return nil
}

func removeTemp(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to remove temporary backup file", "path", path, "error", err)
	}
}

func stageS3(ctx context.Context, rt engine.Runtime, target engine.Container, repo engine.RepoConfig) (*StagedBackup, error) {
	if repo.Bucket == "" {
		return nil, errors.New("s3 repo bucket must be configured")
	}

	tmp, err := os.CreateTemp("", "restore-drill-s3-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary backup file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { removeTemp(tmpPath) }()

	key, err := downloadS3(ctx, repo, tmp)
	closeErr := tmp.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close temporary backup file: %w", closeErr)
	}

	namedPath := filepath.Join(filepath.Dir(tmpPath), filepath.Base(key))
	if filepath.Base(key) != filepath.Base(tmpPath) {
		if err := os.Rename(tmpPath, namedPath); err != nil {
			return nil, fmt.Errorf("rename staged s3 object: %w", err)
		}
		tmpPath = namedPath
	}

	staged, err := stageSource(ctx, rt, target, tmpPath)
	if err != nil {
		return nil, err
	}
	staged.Description = "s3://" + repo.Bucket + "/" + key
	return staged, nil
}

func downloadS3(ctx context.Context, repo engine.RepoConfig, w io.Writer) (string, error) {
	region := repo.Region
	if region == "" {
		region = "us-east-1"
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return "", fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if repo.Endpoint != "" {
			endpoint := repo.Endpoint
			if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
				endpoint = "https://" + endpoint
			}
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		}
	})

	key := strings.TrimPrefix(repo.Prefix, "/")
	if key == "" || strings.HasSuffix(key, "/") {
		latest, err := latestObjectKey(ctx, client, repo.Bucket, key)
		if err != nil {
			return "", err
		}
		key = latest
	}

	resp, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(repo.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", fmt.Errorf("download s3://%s/%s: %w", repo.Bucket, key, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("failed to close s3 response body", "error", err)
		}
	}()

	if _, err := io.Copy(w, resp.Body); err != nil {
		return "", fmt.Errorf("write downloaded s3 object: %w", err)
	}
	slog.Info("downloaded s3 backup object", "bucket", repo.Bucket, "key", key)
	return key, nil
}

func latestObjectKey(ctx context.Context, client *s3.Client, bucket, prefix string) (string, error) {
	var latestKey string
	var latestTime int64
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("list s3://%s/%s: %w", bucket, prefix, err)
		}
		for _, obj := range page.Contents {
			if obj.Key == nil || strings.HasSuffix(*obj.Key, "/") || obj.LastModified == nil {
				continue
			}
			modified := obj.LastModified.UnixNano()
			if latestKey == "" || modified > latestTime {
				latestKey = *obj.Key
				latestTime = modified
			}
		}
	}

	if latestKey == "" {
		return "", fmt.Errorf("no objects found at s3://%s/%s", bucket, prefix)
	}
	return latestKey, nil
}

// ArchiveRequirements returns commands needed inside the target to expand path.
func ArchiveRequirements(path string) []string {
	switch archiveKind(path) {
	case "tar", "tar.gz":
		return []string{"tar"}
	case "xbstream":
		return []string{"xbstream"}
	case "xbstream.gz":
		return []string{"gzip", "xbstream"}
	default:
		return nil
	}
}

// MaterializeArchive expands a staged archive inside target and returns the directory to restore from.
//
// Non-archive paths are returned unchanged so callers can support both mounted
// directories and common physical-backup archive formats with one code path.
func MaterializeArchive(ctx context.Context, rt engine.Runtime, target engine.Container, stagedPath, destDir string) (string, error) {
	kind := archiveKind(stagedPath)
	if kind == "" {
		return stagedPath, nil
	}

	out, err := rt.Exec(ctx, target, []string{"sh", "-c", materializeArchiveScript(stagedPath, destDir, kind)})
	if err != nil {
		return "", fmt.Errorf("extract backup archive %s: %w", stagedPath, err)
	}

	restoreDir := strings.TrimSpace(string(out))
	if restoreDir == "" {
		return "", fmt.Errorf("extract backup archive %s: target extraction command returned an empty path", stagedPath)
	}
	return restoreDir, nil
}

func archiveKind(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".xbstream.gz"):
		return "xbstream.gz"
	case strings.HasSuffix(lower, ".xbstream"):
		return "xbstream"
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	default:
		return ""
	}
}

func materializeArchiveScript(stagedPath, destDir, kind string) string {
	var extract string
	switch kind {
	case "tar.gz":
		extract = `tar -xzf "$src" -C "$dest"`
	case "tar":
		extract = `tar -xf "$src" -C "$dest"`
	case "xbstream.gz":
		extract = `gzip -dc "$src" | xbstream -x -C "$dest"`
	case "xbstream":
		extract = `xbstream -x -C "$dest" < "$src"`
	default:
		extract = `echo "unsupported archive format" >&2; exit 64`
	}

	return fmt.Sprintf(`set -eu
# restore-drill physical backup archive materialization
src=%s
dest=%s
rm -rf "$dest"
mkdir -p "$dest"
%s
if [ -f "$dest/xtrabackup_checkpoints" ] || [ -f "$dest/backup.info" ] || [ -d "$dest/archive" ]; then
  printf '%%s' "$dest"
  exit 0
fi
set -- "$dest"/*
if [ "$#" -eq 1 ] && [ -d "$1" ]; then
  printf '%%s' "$1"
  exit 0
fi
printf '%%s' "$dest"
`, shellQuote(stagedPath), shellQuote(destDir), extract)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
