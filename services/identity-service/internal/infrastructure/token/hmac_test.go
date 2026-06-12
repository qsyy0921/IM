package token

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestHMACSignerSignsPushGatewayCompatibleClaims(t *testing.T) {
	signer, err := NewHMACSigner("secret")
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	token, err := signer.SignGatewayToken(types.TokenClaims{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		DeviceID:  "device-1",
		SessionID: "session-1",
		Audience:  "push-gateway",
		TraceID:   "trace-1",
		ExpiresAt: time.Unix(1_800_000_000, 0).Unix(),
	})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		t.Fatalf("unexpected token format: %s", token)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if claims["tenant_id"] != "tenant-1" ||
		claims["user_id"] != "user-1" ||
		claims["device_id"] != "device-1" ||
		claims["session_id"] != "session-1" ||
		claims["aud"] != "push-gateway" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestJWTSignerSignsStandardGatewayJWT(t *testing.T) {
	signer, err := NewJWTSigner("secret", "kid-1", "issuer-1")
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	token, err := signer.SignGatewayToken(types.TokenClaims{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		DeviceID:  "device-1",
		SessionID: "session-1",
		Audience:  "push-gateway",
		TraceID:   "trace-1",
		IssuedAt:  time.Unix(1_799_999_900, 0).Unix(),
		ExpiresAt: time.Unix(1_800_000_000, 0).Unix(),
	})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		t.Fatalf("unexpected jwt format: %s", token)
	}
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header map[string]any
	if err := json.Unmarshal(headerRaw, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header["alg"] != "HS256" || header["typ"] != "JWT" || header["kid"] != "kid-1" {
		t.Fatalf("unexpected jwt header: %+v", header)
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadRaw, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if claims["iss"] != "issuer-1" ||
		claims["sub"] != "user-1" ||
		claims["tenant_id"] != "tenant-1" ||
		claims["session_id"] != "session-1" ||
		claims["aud"] != "push-gateway" {
		t.Fatalf("unexpected jwt claims: %+v", claims)
	}
}

func TestJWTSignerExposesJWKSet(t *testing.T) {
	signer, err := NewJWTSigner("secret", "kid-1", "issuer-1")
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	jwks := signer.JWKSet()
	if len(jwks.Keys) != 1 {
		t.Fatalf("expected one key, got %+v", jwks)
	}
	key := jwks.Keys[0]
	if key.KeyType != "oct" || key.KeyUse != "sig" || key.KeyID != "kid-1" || key.Algorithm != "HS256" || key.Key == "" {
		t.Fatalf("unexpected jwk: %+v", key)
	}
}

func TestHMACSignerRejectsIncompleteClaims(t *testing.T) {
	signer, err := NewHMACSigner("secret")
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	if _, err := signer.SignGatewayToken(types.TokenClaims{TenantID: "tenant-1", UserID: "user-1"}); err == nil {
		t.Fatal("expected incomplete claims to fail")
	}
}
