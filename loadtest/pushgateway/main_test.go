package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseDeviceIDs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback string
		want     []string
	}{
		{
			name:     "multiple",
			input:    "d1,d2",
			fallback: "fallback",
			want:     []string{"d1", "d2"},
		},
		{
			name:     "trim and dedupe",
			input:    " d1, d1, , d2 ",
			fallback: "fallback",
			want:     []string{"d1", "d2"},
		},
		{
			name:     "empty fallback",
			input:    "",
			fallback: "fallback",
			want:     []string{"fallback"},
		},
		{
			name:     "commas fallback",
			input:    ",,",
			fallback: "fallback",
			want:     []string{"fallback"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseDeviceIDs(test.input, test.fallback)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseDeviceIDs(%q, %q) = %#v, want %#v", test.input, test.fallback, got, test.want)
			}
		})
	}
}

func TestDerivePushMetricsURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "ws", in: "ws://127.0.0.1:11598", want: "http://127.0.0.1:11598/debug/metrics"},
		{name: "wss", in: "wss://push.example/ws?token=x", want: "https://push.example/debug/metrics"},
		{name: "invalid scheme", in: "http://127.0.0.1:11598", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := derivePushMetricsURL(test.in); got != test.want {
				t.Fatalf("derivePushMetricsURL(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestPushAuthTokenSignedWithNonCurrentSecret(t *testing.T) {
	if pushAuthTokenSignedWithNonCurrentSecret(config{
		pushAuthMode:               "hmac",
		pushAuthHMACSecret:         "current",
		pushAuthTokenSigningSecret: "current",
	}) {
		t.Fatalf("expected current signing secret not to be marked as previous")
	}
	if !pushAuthTokenSignedWithNonCurrentSecret(config{
		pushAuthMode:               "hmac",
		pushAuthHMACSecret:         "current",
		pushAuthTokenSigningSecret: "old",
	}) {
		t.Fatalf("expected distinct signing secret to be marked as previous")
	}
}

func TestNormalizePushAuthConfigDefaultsSigningSecretToCurrent(t *testing.T) {
	cfg := config{
		pushAuthMode:       "hmac",
		pushAuthHMACSecret: " current-secret ",
	}
	normalizePushAuthConfig(&cfg)
	if cfg.pushAuthTokenSigningSecret != "current-secret" {
		t.Fatalf("expected signing secret to default to current secret, got %q", cfg.pushAuthTokenSigningSecret)
	}
	if cfg.pushAuthTokenSigningSecretExplicit {
		t.Fatalf("expected defaulted signing secret not to be explicit")
	}
}

func TestNormalizePushAuthConfigPreservesExplicitSigningSecret(t *testing.T) {
	cfg := config{
		pushAuthMode:               "hmac",
		pushAuthHMACSecret:         "current-secret",
		pushAuthTokenSigningSecret: " old-secret ",
	}
	normalizePushAuthConfig(&cfg)
	if cfg.pushAuthTokenSigningSecret != "old-secret" {
		t.Fatalf("expected explicit signing secret, got %q", cfg.pushAuthTokenSigningSecret)
	}
	if !cfg.pushAuthTokenSigningSecretExplicit {
		t.Fatalf("expected explicit signing secret flag")
	}
}

func TestSignPushGatewayTokenUsesTokenSigningSecret(t *testing.T) {
	cfg := config{
		tenantID:                   "tenant-1",
		receiverUserID:             "user-1",
		pushAuthHMACSecret:         "current-secret",
		pushAuthTokenSigningSecret: "old-secret",
		pushAuthTokenTTL:           time.Minute,
	}
	token, err := signPushGatewayToken(cfg, "device-1")
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		t.Fatalf("unexpected token shape: %q", token)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if !validHMAC(parts[0], signature, "old-secret") {
		t.Fatalf("expected token signed by old secret")
	}
	if validHMAC(parts[0], signature, "current-secret") {
		t.Fatalf("did not expect token signed by current secret")
	}
}

func validHMAC(payload string, signature []byte, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return hmac.Equal(signature, mac.Sum(nil))
}
