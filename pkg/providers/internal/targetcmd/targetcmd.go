// Package targetcmd contains helpers for running shell-adjacent commands inside restore targets.
package targetcmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

// ConfiguredBackupPath returns the path-like backup source used for capability checks.
func ConfiguredBackupPath(cfg engine.BackupConfig) string {
	if cfg.Source != "" {
		return cfg.Source
	}
	return cfg.Repo.Prefix
}

// CommandExists checks whether name is available inside the restore target.
func CommandExists(ctx context.Context, rt engine.Runtime, target engine.Container, name string) error {
	if _, err := rt.Exec(ctx, target, []string{"sh", "-c", "command -v " + ShellQuote(name)}); err != nil {
		return fmt.Errorf("required command %q not found in restore image", name)
	}
	return nil
}

// CommandExistsAny checks whether at least one of names is available inside the restore target.
func CommandExistsAny(ctx context.Context, rt engine.Runtime, target engine.Container, label string, names ...string) error {
	if _, err := FirstAvailableCommand(ctx, rt, target, names...); err == nil {
		return nil
	}
	return fmt.Errorf("required %s (%s) not found in restore image", label, strings.Join(names, " or "))
}

// FirstAvailableCommand returns the first command name available inside the restore target.
func FirstAvailableCommand(ctx context.Context, rt engine.Runtime, target engine.Container, names ...string) (string, error) {
	for _, name := range names {
		if _, err := rt.Exec(ctx, target, []string{"sh", "-c", "command -v " + ShellQuote(name)}); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("required command %q not found in restore image", strings.Join(names, " or "))
}

// ShellQuote quotes s for POSIX shell single-token use.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// ShellEscapePostgresLiteral escapes s for use inside a single-quoted PostgreSQL config literal.
func ShellEscapePostgresLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
