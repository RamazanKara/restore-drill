package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// EvalExpression evaluates an expectation expression against an actual value.
// Supported expressions:
//   - "> N", ">= N", "< N", "<= N", "== N" — numeric comparison
//   - "contains \"text\"" — substring match
//   - "age < Xh" / "age < Xm" — time freshness check
//   - "true" / "false" — boolean
//   - literal string comparison (fallback)
func EvalExpression(expect, actual string) (bool, error) {
	expect = strings.TrimSpace(expect)
	actual = strings.TrimSpace(actual)

	// Boolean
	if expect == "true" {
		return isTruthy(actual), nil
	}
	if expect == "false" {
		return actual == "false" || actual == "0" || actual == "f", nil
	}
	if expect == "exists" {
		return isTruthy(actual), nil
	}

	// Age expression: "age < 2h", "age < 30m"
	if strings.HasPrefix(expect, "age") {
		return evalAge(expect, actual)
	}

	// Contains expression: contains "substring"
	if strings.HasPrefix(expect, "contains ") {
		return evalContains(expect, actual)
	}

	// Numeric comparisons: > N, >= N, < N, <= N, == N
	if matched, result, err := evalNumericComparison(expect, actual); matched {
		return result, err
	}

	if strings.Contains(expect, ",") {
		return evalListContains(expect, actual), nil
	}

	// Fallback: exact string match
	return expect == actual, nil
}

func isTruthy(actual string) bool {
	switch strings.ToLower(strings.TrimSpace(actual)) {
	case "true", "1", "t", "yes", "y", "exists":
		return true
	default:
		return false
	}
}

var numericPattern = regexp.MustCompile(`^(>=|<=|>|<|==|!=)\s*(.+)$`)

func evalNumericComparison(expect, actual string) (matched bool, result bool, err error) {
	parts := numericPattern.FindStringSubmatch(expect)
	if parts == nil {
		return false, false, nil
	}

	op := parts[1]
	expectedVal, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	if err != nil {
		return true, false, fmt.Errorf("invalid numeric value in expression %q: %w", expect, err)
	}

	actualVal, err := strconv.ParseFloat(actual, 64)
	if err != nil {
		return true, false, fmt.Errorf("actual value %q is not numeric: %w", actual, err)
	}

	switch op {
	case ">":
		return true, actualVal > expectedVal, nil
	case ">=":
		return true, actualVal >= expectedVal, nil
	case "<":
		return true, actualVal < expectedVal, nil
	case "<=":
		return true, actualVal <= expectedVal, nil
	case "==":
		return true, actualVal == expectedVal, nil
	case "!=":
		return true, actualVal != expectedVal, nil
	default:
		return true, false, fmt.Errorf("unknown operator %q", op)
	}
}

var agePattern = regexp.MustCompile(`^age\s*(>=|<=|>|<)\s*(\d+)(h|m|s)$`)

func evalAge(expect, actual string) (bool, error) {
	parts := agePattern.FindStringSubmatch(expect)
	if parts == nil {
		return false, fmt.Errorf("invalid age expression %q (use: age < 2h)", expect)
	}

	op := parts[1]
	amount, _ := strconv.Atoi(parts[2])
	unit := parts[3]

	var threshold time.Duration
	switch unit {
	case "h":
		threshold = time.Duration(amount) * time.Hour
	case "m":
		threshold = time.Duration(amount) * time.Minute
	case "s":
		threshold = time.Duration(amount) * time.Second
	}

	// Try parsing actual as a timestamp
	actualTime, err := parseTimestamp(actual)
	if err != nil {
		return false, fmt.Errorf("cannot parse %q as timestamp: %w", actual, err)
	}

	age := time.Since(actualTime)

	switch op {
	case "<":
		return age < threshold, nil
	case "<=":
		return age <= threshold, nil
	case ">":
		return age > threshold, nil
	case ">=":
		return age >= threshold, nil
	default:
		return false, fmt.Errorf("unknown age operator %q", op)
	}
}

func parseTimestamp(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999999",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}

	// Try unix timestamp
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(n, 0), nil
	}

	return time.Time{}, fmt.Errorf("unrecognized timestamp format: %s", s)
}

var containsPattern = regexp.MustCompile(`^contains\s+"([^"]*)"$`)

func evalContains(expect, actual string) (bool, error) {
	parts := containsPattern.FindStringSubmatch(expect)
	if parts == nil {
		return false, fmt.Errorf("invalid contains expression %q (use: contains \"text\")", expect)
	}
	return strings.Contains(actual, parts[1]), nil
}

func evalListContains(expect, actual string) bool {
	actualSet := make(map[string]struct{})
	for _, item := range strings.Split(actual, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			actualSet[item] = struct{}{}
		}
	}
	for _, item := range strings.Split(expect, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := actualSet[item]; !ok {
			return false
		}
	}
	return true
}
