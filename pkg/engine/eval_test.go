package engine

import (
	"testing"
	"time"
)

func TestEvalExpression_NumericComparisons(t *testing.T) {
	tests := []struct {
		expect string
		actual string
		want   bool
	}{
		{"> 0", "42", true},
		{"> 0", "0", false},
		{">= 10", "10", true},
		{">= 10", "9", false},
		{"< 100", "50", true},
		{"< 100", "100", false},
		{"<= 100", "100", true},
		{"== 42", "42", true},
		{"== 42", "43", false},
		{"!= 0", "1", true},
		{"!= 0", "0", false},
	}

	for _, tt := range tests {
		t.Run(tt.expect+"_"+tt.actual, func(t *testing.T) {
			got, err := EvalExpression(tt.expect, tt.actual)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("EvalExpression(%q, %q) = %v, want %v", tt.expect, tt.actual, got, tt.want)
			}
		})
	}
}

func TestEvalExpression_Boolean(t *testing.T) {
	tests := []struct {
		expect string
		actual string
		want   bool
	}{
		{"true", "true", true},
		{"true", "1", true},
		{"true", "t", true},
		{"true", "false", false},
		{"exists", "true", true},
		{"exists", "1", true},
		{"false", "false", true},
		{"false", "0", true},
		{"false", "f", true},
		{"false", "true", false},
	}

	for _, tt := range tests {
		t.Run(tt.expect+"_"+tt.actual, func(t *testing.T) {
			got, err := EvalExpression(tt.expect, tt.actual)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("EvalExpression(%q, %q) = %v, want %v", tt.expect, tt.actual, got, tt.want)
			}
		})
	}
}

func TestEvalExpression_ListContains(t *testing.T) {
	tests := []struct {
		expect string
		actual string
		want   bool
	}{
		{"pgcrypto, uuid-ossp", "plpgsql, pgcrypto, uuid-ossp", true},
		{"pgcrypto, missing", "plpgsql, pgcrypto", false},
	}

	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			got, err := EvalExpression(tt.expect, tt.actual)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("EvalExpression(%q, %q) = %v, want %v", tt.expect, tt.actual, got, tt.want)
			}
		})
	}
}

func TestEvalExpression_Contains(t *testing.T) {
	tests := []struct {
		expect string
		actual string
		want   bool
	}{
		{`contains "hello"`, "hello world", true},
		{`contains "missing"`, "hello world", false},
		{`contains ""`, "anything", true},
	}

	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			got, err := EvalExpression(tt.expect, tt.actual)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("EvalExpression(%q, %q) = %v, want %v", tt.expect, tt.actual, got, tt.want)
			}
		})
	}
}

func TestEvalExpression_Age(t *testing.T) {
	recent := time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
	old := time.Now().Add(-3 * time.Hour).Format(time.RFC3339)

	tests := []struct {
		expect string
		actual string
		want   bool
	}{
		{"age < 2h", recent, true},
		{"age < 2h", old, false},
		{"age > 1h", old, true},
		{"age > 1h", recent, false},
	}

	for _, tt := range tests {
		t.Run(tt.expect+"_"+tt.actual, func(t *testing.T) {
			got, err := EvalExpression(tt.expect, tt.actual)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("EvalExpression(%q, %q) = %v, want %v", tt.expect, tt.actual, got, tt.want)
			}
		})
	}
}

func TestEvalExpression_ExactMatch(t *testing.T) {
	got, err := EvalExpression("hello", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected exact match to pass")
	}

	got, err = EvalExpression("hello", "world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected mismatch to fail")
	}
}

func TestEvalExpression_Errors(t *testing.T) {
	tests := []struct {
		name   string
		expect string
		actual string
	}{
		{"non-numeric actual", "> 10", "not_a_number"},
		{"invalid age format", "age < 2h", "not_a_timestamp"},
		{"invalid contains syntax", `contains no_quotes`, "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := EvalExpression(tt.expect, tt.actual)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
