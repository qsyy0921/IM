package main

import "testing"

func TestSanitizeRunName(t *testing.T) {
	if got := sanitizeRunName("Model Gateway Smoke_01"); got != "model-gateway-smoke-01" {
		t.Fatalf("sanitizeRunName = %q", got)
	}
}

func TestHashRef(t *testing.T) {
	if got := hashRef("hello"); got == "" || got[:7] != "sha256:" {
		t.Fatalf("unexpected hash ref: %q", got)
	}
}
