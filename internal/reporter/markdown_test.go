package reporter

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/internal/engine"
	"github.com/RamazanKara/restore-drill/internal/state"
)

func TestMarkdownResults(t *testing.T) {
	tests := []struct {
		name    string
		results []engine.DrillResult
		want    []string
	}{
		{"empty", nil, []string{"# Restore drill results", "No drills recorded."}},
		{"pass", []engine.DrillResult{{Name: "redis", Provider: "redis", Duration: time.Second, ValidationPassed: true}}, []string{"| redis | redis | pass | 1s |", "No checks recorded."}},
		{"restore failure", []engine.DrillResult{{Name: "redis", Error: errors.New("restore failed")}}, []string{"| fail |", "Error: restore failed"}},
		{"check failure", []engine.DrillResult{{Name: "redis", Checks: []engine.CheckResult{{Name: "count", Type: "query", Expected: "> 0", Actual: "0", Duration: time.Millisecond, Error: errors.New("empty")}}}}, []string{"| count | query | FAIL | &gt; 0 | 0 | 1ms | empty |"}},
		{"retained target", []engine.DrillResult{{Name: "redis", CleanupSkipped: true, TargetID: "target-1", TargetHost: "127.0.0.1", TargetPorts: map[int]int{6379: 16379}, BackupTimestamp: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC), BackupAge: time.Hour}}, []string{"Retained target: target-1; host: 127.0.0.1", "Port 6379 → 16379", "Backup timestamp: 2026-05-20T12:00:00Z; backup age: 1h0m0s"}},
		{"escaping", []engine.DrillResult{{Name: "a|b", Error: errors.New("<script>\n**bad** [link](url)")}}, []string{"a&#124;b", "&lt;script&gt;<br>\\*\\*bad\\*\\* \\[link\\](url)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := (&Markdown{Writer: &buf}).Report(context.Background(), tt.results); err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.want {
				if !strings.Contains(buf.String(), want) {
					t.Fatalf("missing %q:\n%s", want, buf.String())
				}
			}
		})
	}
}

func TestRenderMarkdownEvidence(t *testing.T) {
	ts := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "pass", true: "fail"}[failed], func(t *testing.T) {
			result := state.RunResult{Name: "redis", Provider: "redis", Duration: "2s", ValidationPassed: !failed}
			if failed {
				result.Error = "restore failed"
				result.Checks = []state.CheckResult{{Name: "count", Type: "query", Expected: "> 0", Actual: "0|<script>\r\nnext"}}
			}
			report := BuildEvidenceReport([]*state.LastRun{{Timestamp: ts, Results: []state.RunResult{result}}}, ts.Add(-time.Hour))
			var buf bytes.Buffer
			if err := RenderMarkdown(&buf, report); err != nil {
				t.Fatal(err)
			}
			want := []string{"# Backup Restore Verification Evidence", "Evidence Checks", "Drill History", "Avg RTO", "| redis | redis | 1 |", "2026-05-20T12:00:00Z"}
			if failed {
				want = append(want, "Failure Evidence", "restore failed", "| count | query | &gt; 0 | 0&#124;&lt;script&gt;<br>next |")
			} else {
				want = append(want, "100.0%", "No failure details recorded.")
			}
			for _, text := range want {
				if !strings.Contains(buf.String(), text) {
					t.Fatalf("missing %q:\n%s", text, buf.String())
				}
			}
		})
	}
}

func TestMarkdownHistoryOrder(t *testing.T) {
	report := &EvidenceReport{DrillSummary: []DrillSummary{{Name: "zebra"}, {Name: "alpha"}}}
	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, report); err != nil {
		t.Fatal(err)
	}
	if strings.Index(buf.String(), "| alpha |") > strings.Index(buf.String(), "| zebra |") {
		t.Fatal("history is not sorted")
	}
	if report.DrillSummary[0].Name != "zebra" {
		t.Fatal("renderer mutated its input")
	}
}

type markdownErrorWriter struct{}

func (markdownErrorWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestMarkdownWriterErrors(t *testing.T) {
	tests := []struct {
		name   string
		render func(io.Writer) error
	}{
		{"run", func(w io.Writer) error { return (&Markdown{Writer: w}).Report(context.Background(), nil) }},
		{"history", func(w io.Writer) error { return RenderMarkdown(w, &EvidenceReport{}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.render(markdownErrorWriter{}); !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("got %v, want writer error", err)
			}
		})
	}
}

func FuzzEscapeMarkdown(f *testing.F) {
	for _, input := range []string{"", "|<script>", "a\r\nb\rc\nd", "\\|`_*[]#~!", "&lt;"} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		got := escapeMarkdown(input)
		if strings.ContainsAny(got, "|\r\n") {
			t.Fatalf("unescaped table separator: %q", got)
		}
		if strings.ContainsAny(strings.ReplaceAll(got, "<br>", ""), "<>") {
			t.Fatalf("unescaped HTML: %q", got)
		}
	})
}
