package main

import (
	"testing"
)

func TestAddNumbers(t *testing.T) {
	got, err := addNumbers(nil, addInput{A: 17, B: 25})
	if err != nil || got.Sum != 42 {
		t.Fatalf("addNumbers(17, 25) = %+v, %v; want 42", got, err)
	}
}

func TestRunRequiresCompiledConfig(t *testing.T) {
	t.Setenv("KAGENT_CONFIG_JSON", "")
	if err := run(t.TempDir()); err == nil {
		t.Fatal("run() without KAGENT_CONFIG_JSON succeeded; want an error")
	}
}
