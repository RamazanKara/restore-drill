package engine

import "testing"

func FuzzExpressionAndValues(f *testing.F) {
	for _, seed := range [][2]string{{"> 0", "12"}, {"age < 24h", "2026-05-20T12:00:00Z"}, {`contains "PONG"`, "PONG"}, {"true", "yes"}, {"a,b", "a,b,c"}, {">= bad", "512Mi"}, {"", "500m"}} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, expression, actual string) {
		passed, err := EvalExpression(expression, actual)
		if err != nil && passed {
			t.Fatal("invalid expression passed validation")
		}
		_, _ = parseTimestamp(actual)
		_ = parseMemory(actual)
		_ = parseCPU(actual)
	})
}
