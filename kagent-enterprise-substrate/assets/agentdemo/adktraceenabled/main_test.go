package main

import (
	"strings"
	"testing"
)

func TestAddNumbers(t *testing.T) {
	got, err := addNumbers(nil, addInput{A: 17, B: 25})
	if err != nil || got.Sum != 42 {
		t.Fatalf("addNumbers(17, 25) = %+v, %v; want 42", got, err)
	}
}

func TestRunRequiresTraceConfiguration(t *testing.T) {
	t.Setenv("KAGENT_NAME", "adk-trace-enabled-adk-trace-enabled")
	t.Setenv("KAGENT_NAMESPACE", "kagent")
	t.Setenv("OTEL_TRACING_ENABLED", "false")
	if err := run(); err == nil || !strings.Contains(err.Error(), "OTEL_TRACING_ENABLED") {
		t.Fatalf("run() error = %v; want missing trace configuration", err)
	}
}
