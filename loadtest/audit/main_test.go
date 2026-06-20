package main

import (
	"path/filepath"
	"testing"
)

func TestSanitizeRunName(t *testing.T) {
	if got := sanitizeRunName(" audit smoke/one "); got != "audit-smoke-one" {
		t.Fatalf("unexpected sanitized run name %q", got)
	}
}

func TestPayloadLeaksSensitiveValue(t *testing.T) {
	if !payloadLeaksSensitiveValue(`{"source_ref":"raw_prompt"}`) {
		t.Fatal("raw prompt marker should be treated as sensitive")
	}
	if payloadLeaksSensitiveValue(`{"audit_id":"aud_1","record_hash":"abc"}`) {
		t.Fatal("low-sensitive audit payload should be allowed")
	}
}

func TestPathInside(t *testing.T) {
	repoRoot := `E:\development\IM`
	repoLocalResultPath := filepath.Join(repoRoot, "loadtest", "results")
	if !pathInside(repoLocalResultPath, repoRoot) {
		t.Fatal("repo-local result path should be detected")
	}
	if pathInside(`H:\NexusIM\loadtest-results\audit`, repoRoot) {
		t.Fatal("external H drive result path should be allowed")
	}
}
