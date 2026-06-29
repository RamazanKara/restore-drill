package engine

import "testing"

func TestParseCPU(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"500m", 500_000_000},
		{"1", 1_000_000_000},
		{"1.5", 1_500_000_000},
		{"bad", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := parseCPU(tt.input); got != tt.want {
				t.Fatalf("parseCPU(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseMemory(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"512Mi", 512 * 1024 * 1024},
		{"2Gi", 2 * 1024 * 1024 * 1024},
		{"1G", 1_000_000_000},
		{"bad", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := parseMemory(tt.input); got != tt.want {
				t.Fatalf("parseMemory(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
