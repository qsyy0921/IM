package main

import (
	"path/filepath"
	"testing"
)

func TestSanitizeRunName(t *testing.T) {
	got := sanitizeRunName(" media smoke/../run 01 ")
	if got != "media-smoke----run-01" {
		t.Fatalf("unexpected sanitized run name: %q", got)
	}
}

func TestPathInside(t *testing.T) {
	root := filepath.Join("E:", "development", "IM")
	inside := filepath.Join(root, "loadtest", "results")
	outside := filepath.Join("H:", "NexusIM", "loadtest-results")
	if !pathInside(inside, root) {
		t.Fatalf("expected inside path to be detected")
	}
	if pathInside(outside, root) {
		t.Fatalf("expected external path to be allowed")
	}
}

func TestURLSafety(t *testing.T) {
	for _, value := range []string{
		"http://media.local/fake?op=get&token=abc",
		"http://media.local/fake?token=abc",
	} {
		if !isURLSafe(value) {
			t.Fatalf("expected URL to be safe: %s", value)
		}
	}
	for _, value := range []string{
		"http://media.local/fake?key=tenant/object",
		"http://media.local/fake?object_key=tenant/object",
		"",
	} {
		if isURLSafe(value) {
			t.Fatalf("expected URL to be unsafe: %s", value)
		}
	}
}
