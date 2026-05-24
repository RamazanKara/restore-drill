package main

import "testing"

func TestParseKeyValueFlags(t *testing.T) {
	values, err := parseKeyValueFlags([]string{"team=platform", "env=prod"}, "--kube-pod-label")
	if err != nil {
		t.Fatalf("parse key-value flags: %v", err)
	}
	if values["team"] != "platform" {
		t.Fatalf("expected team label, got %#v", values)
	}
	if values["env"] != "prod" {
		t.Fatalf("expected env label, got %#v", values)
	}
}

func TestParseKeyValueFlagsRejectsMissingValueSeparator(t *testing.T) {
	_, err := parseKeyValueFlags([]string{"team"}, "--kube-pod-label")
	if err == nil {
		t.Fatal("expected invalid key-value flag to fail")
	}
}
