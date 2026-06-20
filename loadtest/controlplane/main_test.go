package main

import "testing"

func TestPayloadLeaksSensitiveValue(t *testing.T) {
	for _, payload := range []string{
		`{"provider_token":"x"}`,
		`{"requests_per_second":20}`,
		`{"payload_json":"{}"}`,
	} {
		if !payloadLeaksSensitiveValue(payload) {
			t.Fatalf("expected sensitive payload: %s", payload)
		}
	}
	if payloadLeaksSensitiveValue(`{"checksum_present":true,"bundle_key":"api-gateway/default"}`) {
		t.Fatal("low-sensitive payload should pass")
	}
}

func TestSanitizeRunName(t *testing.T) {
	if got := sanitizeRunName(`control plane: smoke`); got != "control-plane--smoke" {
		t.Fatalf("unexpected sanitized name %q", got)
	}
}
