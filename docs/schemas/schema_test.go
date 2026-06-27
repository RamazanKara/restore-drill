package schemas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/pkg/engine"
	"github.com/RamazanKara/restore-drill/pkg/reporter"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

func TestConfigSchemaValidatesExamples(t *testing.T) {
	schema := compileSchema(t, "config-v1.schema.json")
	examples, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.yaml"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(examples) == 0 {
		t.Fatal("expected example configs")
	}

	for _, path := range examples {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read example: %v", err)
			}
			var doc any
			if err := yaml.Unmarshal(data, &doc); err != nil {
				t.Fatalf("parse example yaml: %v", err)
			}
			normalized := normalizeYAML(doc)
			if err := schema.Validate(normalized); err != nil {
				t.Fatalf("schema validation failed: %v", err)
			}
		})
	}
}

func TestRunResultSchemaValidatesReporterOutput(t *testing.T) {
	schema := compileSchema(t, "run-result-v1.schema.json")
	start := time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)
	results := []engine.DrillResult{
		{
			Name:             "production-postgres",
			Provider:         "postgres",
			StartedAt:        start,
			Duration:         1500 * time.Millisecond,
			BackupTimestamp:  start.Add(-2 * time.Hour),
			BackupAge:        2 * time.Hour,
			ValidationPassed: true,
			CleanupSkipped:   true,
			TargetID:         "restore-drill/target",
			TargetHost:       "127.0.0.1",
			TargetPorts:      map[int]int{5432: 15432},
			Checks: []engine.CheckResult{
				{
					Name:     "users-exist",
					Type:     "query",
					Expected: "> 0",
					Actual:   "42",
					Passed:   true,
					Duration: 25 * time.Millisecond,
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := (&reporter.JSON{Writer: &buf, Pretty: true}).Report(context.Background(), results); err != nil {
		t.Fatalf("render reporter json: %v", err)
	}
	var doc any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("parse reporter json: %v", err)
	}
	if err := schema.Validate(doc); err != nil {
		t.Fatalf("schema validation failed: %v\njson:\n%s", err, buf.String())
	}
}

func compileSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	path := filepath.Join(name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema %s: %v", name, err)
	}
	if err := compiler.AddResource(name, bytes.NewReader(data)); err != nil {
		t.Fatalf("add schema %s: %v", name, err)
	}
	schema, err := compiler.Compile(name)
	if err != nil {
		t.Fatalf("compile schema %s: %v", name, err)
	}
	return schema
}

func normalizeYAML(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, val := range typed {
			out[k] = normalizeYAML(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(typed))
		for k, val := range typed {
			out[k.(string)] = normalizeYAML(val)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, val := range typed {
			out[i] = normalizeYAML(val)
		}
		return out
	default:
		return typed
	}
}
