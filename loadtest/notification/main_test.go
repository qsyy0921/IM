package main

import (
	"path/filepath"
	"testing"
)

func TestSanitizeRunName(t *testing.T) {
	got := sanitizeRunName(" notification smoke/../run 01 ")
	if got != "notification-smoke----run-01" {
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

func TestPayloadSafety(t *testing.T) {
	for _, value := range []string{
		`{"tenant_id":"tenant","request_id":"notif_1","destination_masked":"n***@example.com"}`,
		`{"template_key":"identity.challenge","recipient_ref":"user:123"}`,
	} {
		if !payloadSafe(value) {
			t.Fatalf("expected payload to be safe: %s", value)
		}
	}
	for _, value := range []string{
		`{"destination_ref":"alice@example.com"}`,
		`{"destination_hash":"hash"}`,
		`{"secret_payload_ciphertext":"abc"}`,
		`{"provider_response":"raw"}`,
		`{"reset_token":"raw"}`,
	} {
		if payloadSafe(value) {
			t.Fatalf("expected payload to be unsafe: %s", value)
		}
	}
}
