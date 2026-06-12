package token

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestRS256SignerSignsGatewayJWTAndExposesPublicJWK(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	signer, err := NewRS256SignerFromPEM(testRSAPrivateKeyPEM(privateKey), "kid-rsa-1", "issuer-1")
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
	if header["alg"] != "RS256" || header["typ"] != "JWT" || header["kid"] != "kid-rsa-1" {
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
		claims["device_id"] != "device-1" ||
		claims["aud"] != "push-gateway" {
		t.Fatalf("unexpected jwt claims: %+v", claims)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&privateKey.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
		t.Fatalf("verify signature: %v", err)
	}

	jwks := signer.JWKSet()
	if len(jwks.Keys) != 1 {
		t.Fatalf("expected one key, got %+v", jwks)
	}
	key := jwks.Keys[0]
	if key.KeyType != "RSA" || key.KeyUse != "sig" || key.KeyID != "kid-rsa-1" || key.Algorithm != "RS256" || key.Modulus == "" || key.Exponent == "" {
		t.Fatalf("unexpected jwk: %+v", key)
	}
	if key.Key != "" {
		t.Fatalf("rsa public jwk must not expose symmetric/private key material: %+v", key)
	}
}

func TestRS256SignerRejectsWeakPrivateKey(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	if _, err := NewRS256SignerFromPEM(testRSAPrivateKeyPEM(privateKey), "weak", "issuer-1"); err == nil {
		t.Fatal("expected weak rsa private key to be rejected")
	}
}

func testRSAPrivateKeyPEM(privateKey *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
}
