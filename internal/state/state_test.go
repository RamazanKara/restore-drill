package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSaveAtomicallyReplacesInvalidState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "last-run.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatalf("seed invalid state: %v", err)
	}

	run := &LastRun{
		Timestamp: time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC),
		Results: []RunResult{
			{
				Name:             "postgres",
				Provider:         "postgres",
				ValidationPassed: true,
			},
		},
	}
	if err := Save(path, run); err != nil {
		t.Fatalf("save state: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Name != "postgres" {
		t.Fatalf("unexpected state: %#v", got)
	}
}

func TestAppendHistoryConcurrentSameTimestampKeepsEveryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	const runs = 32
	ts := time.Date(2026, 5, 20, 14, 30, 0, 123, time.UTC)

	var wg sync.WaitGroup
	errs := make(chan error, runs)
	for i := 0; i < runs; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- AppendHistory(&LastRun{
				Timestamp: ts,
				Results: []RunResult{
					{
						Name:             fmt.Sprintf("drill-%02d", i),
						Provider:         "postgres",
						ValidationPassed: true,
					},
				},
			})
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("append history: %v", err)
		}
	}

	entries, err := os.ReadDir(HistoryDir())
	if err != nil {
		t.Fatalf("read history dir: %v", err)
	}
	if len(entries) != runs {
		t.Fatalf("expected %d history files, got %d", runs, len(entries))
	}

	loaded, err := LoadHistory(time.Time{})
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	if len(loaded) != runs {
		t.Fatalf("expected %d loaded runs, got %d", runs, len(loaded))
	}
}

func TestLoadHistorySkipsMalformedEntriesAndSortsRuns(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := HistoryDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create history dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write malformed history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0o600); err != nil {
		t.Fatalf("write ignored history file: %v", err)
	}

	older := &LastRun{
		Timestamp: time.Date(2026, 5, 20, 14, 0, 0, 0, time.UTC),
		Results: []RunResult{
			{Name: "older", Provider: "postgres", ValidationPassed: true},
		},
	}
	newer := &LastRun{
		Timestamp: time.Date(2026, 5, 20, 15, 0, 0, 0, time.UTC),
		Results: []RunResult{
			{Name: "newer", Provider: "redis", ValidationPassed: true},
		},
	}
	if err := Save(filepath.Join(dir, "newer.json"), newer); err != nil {
		t.Fatalf("write newer history: %v", err)
	}
	if err := Save(filepath.Join(dir, "older.json"), older); err != nil {
		t.Fatalf("write older history: %v", err)
	}

	loaded, err := LoadHistory(time.Date(2026, 5, 20, 13, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 valid history entries, got %d", len(loaded))
	}
	if loaded[0].Results[0].Name != "older" || loaded[1].Results[0].Name != "newer" {
		t.Fatalf("history was not sorted by timestamp: %#v", loaded)
	}
}
