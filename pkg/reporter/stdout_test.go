package reporter

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

func TestStdoutIncludesTopLevelErrorWithoutChecks(t *testing.T) {
	var buf bytes.Buffer
	reporter := &Stdout{Writer: &buf}

	results := []engine.DrillResult{
		{
			Name:             "postgres-prod",
			Provider:         "postgres",
			Duration:         time.Second,
			ValidationPassed: true,
			Error:            errors.New("cleanup provider: remove temp data"),
		},
	}

	if err := reporter.Report(context.Background(), results); err != nil {
		t.Fatalf("report stdout: %v", err)
	}

	output := buf.String()
	for _, want := range []string{
		"postgres-prod",
		"FAIL",
		"error: cleanup provider: remove temp data",
		"Result: FAILED",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, output)
		}
	}
}
