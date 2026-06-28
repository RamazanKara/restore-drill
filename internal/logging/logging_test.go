package logging

import (
	"context"
	"log/slog"
	"testing"
)

func TestSetupLevelGating(t *testing.T) {
	tests := []struct {
		name      string
		verbose   bool
		wantDebug bool
		wantInfo  bool
	}{
		{name: "default hides debug", verbose: false, wantDebug: false, wantInfo: true},
		{name: "verbose enables debug", verbose: true, wantDebug: true, wantInfo: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Setup(tt.verbose)

			logger := slog.Default()
			if got := logger.Enabled(context.Background(), slog.LevelDebug); got != tt.wantDebug {
				t.Errorf("debug enabled = %v, want %v", got, tt.wantDebug)
			}
			if got := logger.Enabled(context.Background(), slog.LevelInfo); got != tt.wantInfo {
				t.Errorf("info enabled = %v, want %v", got, tt.wantInfo)
			}
		})
	}
}

func TestSetupReplacesDefaultLogger(t *testing.T) {
	before := slog.Default()
	Setup(true)
	if slog.Default() == before {
		t.Fatal("Setup did not install a new default logger")
	}
}
