// Package logging configures structured logging via log/slog.
package logging

import (
	"log/slog"
	"os"
)

// Setup configures the global slog logger based on verbosity.
func Setup(verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}
