package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const diagnosticConfig = `drills:
  - name: redis
    provider: redis
    backup:
      tool: aof
      source: /backups/appendonly.aof
    restore:
      timeout: 1m
      container:
        image: redis:7-alpine
    checks:
      - name: ping
        type: query
        sql: PING
        expect: PONG
    alerts:
      - type: webhook
        url: https://example.invalid/hook
reporting:
  format: [json, markdown]
  retention: 7d
`

func TestConfigErrorLocations(t *testing.T) {
	tests := []struct {
		name        string
		old         string
		replacement string
		line        int
		field       string
	}{
		{"provider", "provider: redis", "provider: missing", 3, "drills[0].provider"},
		{"missing provider", "    provider: redis\n", "", 2, "drills[0].provider"},
		{"tool", "tool: aof", "tool: pg_dump", 5, "drills[0].backup.tool"},
		{"source", "source: /backups/appendonly.aof", "source: ''", 6, "drills[0].backup.source"},
		{"image", "image: redis:7-alpine", "image: ''", 10, "drills[0].restore.container.image"},
		{"timeout", "timeout: 1m", "timeout: bad", 8, "drills[0].restore.timeout"},
		{"check name", "name: ping", "name: ''", 12, "drills[0].checks[0].name"},
		{"check type", "type: query", "type: schema", 13, "drills[0].checks[0].type"},
		{"check sql", "sql: PING", "sql: ''", 14, "drills[0].checks[0].sql"},
		{"missing expectation", "        expect: PONG\n", "", 12, "drills[0].checks[0].expect"},
		{"alert", "type: webhook", "type: invalid", 17, "drills[0].alerts[0].type"},
		{"alert condition", "url: https://example.invalid/hook", "on: sometimes", 18, "drills[0].alerts[0].on"},
		{"alert url", "url: https://example.invalid/hook", "url: ''", 18, "drills[0].alerts[0].url"},
		{"format", "format: [json, markdown]", "format: [json, pdf]", 20, "reporting.format[1]"},
		{"retention", "retention: 7d", "retention: 0d", 21, "reporting.retention"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.Replace(diagnosticConfig, tt.old, tt.replacement, 1)
			_, err := ParseConfig([]byte(input))
			want := fmt.Sprintf("line %d (%s)", tt.line, tt.field)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("expected %q, got %v", want, err)
			}
		})
	}
}

func TestConfigLocationsWithYAMLFeatures(t *testing.T) {
	tests := []struct{ name, input, want string }{
		{"empty", "", "line 1 (drills)"},
		{"flow", "drills: [{name: x, provider: bad}]", "line 1 (drills[0].provider)"},
		{"alias", "provider: &p bad\ndrills:\n  - name: x\n    provider: *p\n", "line 4 (drills[0].provider)"},
		{"merge", "defaults: &defaults\n  provider: bad\ndrills:\n  - <<: *defaults\n    name: x\n", "line 2 (drills[0].provider)"},
		{"merge sequence", "a: &a {schedule: manual}\nb: &b {provider: bad}\ndrills:\n  - <<: [*a, *b]\n    name: x\n", "line 2 (drills[0].provider)"},
		{"merge override", "a: &a {provider: redis}\ndrills:\n  - <<: *a\n    name: x\n    provider: bad\n", "line 5 (drills[0].provider)"},
		{"duplicate name", "drills:\n  - &drill {name: x, provider: redis, backup: {tool: aof, source: /x}, restore: {container: {image: redis}}}\n  - <<: *drill\n    name: x\n", "line 4 (drills[1].name)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q, got %v", tt.want, err)
			}
		})
	}
}

func TestConfigLocationAfterEnvironmentExpansion(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n", "\r", "\u0085", "\u2028", "\u2029"} {
		t.Run(fmt.Sprintf("%q", newline), func(t *testing.T) {
			t.Setenv("RESTORE_DRILL_TEST_LINES", "# first"+newline+"# second")
			input := "${RESTORE_DRILL_TEST_LINES}" + newline + "drills:" + newline + "  - name: x" + newline + "    provider: bad" + newline
			_, err := ParseConfig([]byte(input))
			if err == nil || !strings.Contains(err.Error(), "line 4 (drills[0].provider)") {
				t.Fatalf("expected original line 4, got %v", err)
			}
		})
	}
}

func TestLoadConfigIncludesFilename(t *testing.T) {
	for _, input := range []string{"drills: []", "drills: [", "drills: wrong-type"} {
		t.Run(input, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "drill.yaml")
			if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadConfig(path)
			if err == nil || !strings.Contains(err.Error(), path+":") || !strings.Contains(err.Error(), "line 1") {
				t.Fatalf("expected filename and line, got %v", err)
			}
		})
	}
}

func FuzzParseConfig(f *testing.F) {
	for _, input := range []string{diagnosticConfig, "", "drills: []", "drills: [", "drills: &a [*a]", "drills:\r  - name: x\r    provider: bad", "defaults: &a {provider: redis}\ndrills: [{<<: *a, name: x}]"} {
		f.Add([]byte(input))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		cfg, err := ParseConfig(data)
		if err != nil {
			if cfg != nil {
				t.Fatal("invalid config returned with an error")
			}
			return
		}
		if cfg == nil {
			t.Fatal("nil config without error")
		}
		if _, err := validateConfig(cfg); err != nil {
			t.Fatalf("accepted invalid config: %v", err)
		}
	})
}

func FuzzInterpolateEnv(f *testing.F) {
	for _, input := range []string{"", "${MISSING:-value}", "a\r\nb", "${MISSING:-a\nb}", "\u0085\u2028\u2029"} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		expanded, lines := interpolateEnv(input)
		if len(lines) != len(yamlLineBreaks.FindAllStringIndex(expanded, -1))+1 {
			t.Fatal("incorrect expanded line count")
		}
		limit := len(yamlLineBreaks.FindAllStringIndex(input, -1)) + 1
		for _, line := range lines {
			if line < 1 || line > limit {
				t.Fatalf("source line %d outside 1..%d", line, limit)
			}
		}
	})
}
