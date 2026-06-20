package main

import "testing"

func TestPayloadLeaksSensitiveValue(t *testing.T) {
	for _, payload := range []string{
		`{"user_id":"user-1"}`,
		`{"device_id":"device-1"}`,
		`{"manual_status":"available"}`,
		`{"password":"x"}`,
	} {
		if !payloadLeaksSensitiveValue(payload) {
			t.Fatalf("expected sensitive payload: %s", payload)
		}
	}
	if payloadLeaksSensitiveValue(`{"tenant_ref":"sha256:abc","user_ref":"sha256:def","state":"ONLINE"}`) {
		t.Fatal("low-sensitive payload should pass")
	}
}

func TestSanitizeRunName(t *testing.T) {
	if got := sanitizeRunName(`presence: smoke`); got != "presence--smoke" {
		t.Fatalf("unexpected sanitized name %q", got)
	}
}
