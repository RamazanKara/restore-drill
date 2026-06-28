package version

import (
	"strings"
	"testing"
)

func TestStringIncludesAllBuildMetadata(t *testing.T) {
	origVersion, origCommit, origDate := Version, Commit, Date
	t.Cleanup(func() {
		Version, Commit, Date = origVersion, origCommit, origDate
	})

	Version = "1.2.3"
	Commit = "abc1234"
	Date = "2026-06-28T00:00:00Z"

	got := String()
	for _, want := range []string{Version, Commit, Date} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, missing %q", got, want)
		}
	}
}

func TestStringDefaults(t *testing.T) {
	// Package defaults are used when ldflags are not applied (e.g. go install
	// without build metadata). They must remain non-empty so the version
	// command always prints something meaningful.
	if Version == "" || Commit == "" || Date == "" {
		t.Fatalf("default build metadata must be non-empty: version=%q commit=%q date=%q", Version, Commit, Date)
	}

	got := String()
	if !strings.Contains(got, "commit:") || !strings.Contains(got, "built:") {
		t.Errorf("String() = %q, expected commit/built labels", got)
	}
}
