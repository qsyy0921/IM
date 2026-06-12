package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	tokeninfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/token"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestGatewayTokenJWKSetWithAdditionalKeysMergesAndDeduplicates(t *testing.T) {
	current := testGatewayRSAJWK(t, generateGatewayTestRSAKey(t), "current")
	old := testGatewayRSAJWK(t, generateGatewayTestRSAKey(t), "old")
	duplicateCurrent := testGatewayRSAJWK(t, generateGatewayTestRSAKey(t), "current")
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_JSON", testGatewayJWKSetJSON(t, old, duplicateCurrent))
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_FILE", "")
	base := tokeninfra.JWKSet{Keys: []tokeninfra.JWK{current}}

	merged, err := gatewayTokenJWKSetWithAdditionalKeys(base)
	if err != nil {
		t.Fatalf("merge jwks: %v", err)
	}
	if len(merged.Keys) != 2 {
		t.Fatalf("expected current plus one old key, got %+v", merged.Keys)
	}
	if merged.Keys[0].KeyID != "current" || merged.Keys[0].Modulus != current.Modulus {
		t.Fatalf("expected base current key to stay first and win duplicates, got %+v", merged.Keys[0])
	}
	if merged.Keys[1].KeyID != "old" {
		t.Fatalf("expected old key to be appended, got %+v", merged.Keys[1])
	}
}

func TestGatewayTokenJWKSetWithAdditionalKeysRejectsSymmetricKeys(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_JSON", `{"keys":[{"kty":"oct","use":"sig","kid":"shared","alg":"HS256","k":"secret"}]}`)
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_FILE", "")
	base := tokeninfra.JWKSet{Keys: []tokeninfra.JWK{{
		KeyType:   "RSA",
		KeyUse:    "sig",
		KeyID:     "current",
		Algorithm: "RS256",
		Modulus:   "base",
		Exponent:  "AQAB",
	}}}

	if _, err := gatewayTokenJWKSetWithAdditionalKeys(base); err == nil {
		t.Fatal("expected symmetric jwk to be rejected")
	}
}

func TestGatewayTokenJWKSetWithAdditionalKeysRejectsWeakRSAKeys(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_JSON", `{"keys":[{"kty":"RSA","use":"sig","kid":"weak","alg":"RS256","n":"abc","e":"AQAB"}]}`)
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_FILE", "")

	if _, err := gatewayTokenJWKSetWithAdditionalKeys(tokeninfra.JWKSet{}); err == nil {
		t.Fatal("expected weak rsa jwk to be rejected")
	}
}

func TestGatewayTokenJWKSetWithAdditionalKeysAllowsNoPublicKeys(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_JSON", "")
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_FILE", "")

	merged, err := gatewayTokenJWKSetWithAdditionalKeys(tokeninfra.JWKSet{})
	if err != nil {
		t.Fatalf("merge empty jwks: %v", err)
	}
	if len(merged.Keys) != 0 {
		t.Fatalf("expected no public keys, got %+v", merged)
	}
}

func TestLoadAdditionalGatewayTokenJWKSetReadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jwks.json")
	old := testGatewayRSAJWK(t, generateGatewayTestRSAKey(t), "old")
	if err := os.WriteFile(path, []byte(testGatewayJWKSetJSON(t, old)), 0o600); err != nil {
		t.Fatalf("write jwks file: %v", err)
	}
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_JSON", "")
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_FILE", path)

	set, err := loadAdditionalGatewayTokenJWKSet()
	if err != nil {
		t.Fatalf("load jwks file: %v", err)
	}
	if len(set.Keys) != 1 || set.Keys[0].KeyID != "old" {
		t.Fatalf("unexpected jwks: %+v", set)
	}
}

func TestLoadAdditionalGatewayTokenJWKSetRejectsInvalidJSON(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_JSON", "{")
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_FILE", "")

	if _, err := loadAdditionalGatewayTokenJWKSet(); err == nil {
		t.Fatalf("expected invalid additional jwks json to fail")
	}
}

func generateGatewayTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return key
}

func testGatewayRSAJWK(t *testing.T, privateKey *rsa.PrivateKey, keyID string) tokeninfra.JWK {
	t.Helper()
	return tokeninfra.JWK{
		KeyType:   "RSA",
		KeyUse:    "sig",
		KeyID:     keyID,
		Algorithm: "RS256",
		Modulus:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
		Exponent:  base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.PublicKey.E)).Bytes()),
	}
}

func testGatewayJWKSetJSON(t *testing.T, keys ...tokeninfra.JWK) string {
	t.Helper()
	raw, err := json.Marshal(tokeninfra.JWKSet{Keys: keys})
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}
	return string(raw)
}

func TestDisabledMFASecretManagerReturnsMFAUnavailable(t *testing.T) {
	manager := disabledMFASecretManager{}
	if _, _, err := manager.NewTOTPSecret(); !errors.Is(err, types.ErrMFAUnavailable) {
		t.Fatalf("expected new totp to return mfa unavailable, got %v", err)
	}
	if _, err := manager.VerifyTOTP(types.EncryptedMFASecret{}, "123456", time.Now()); !errors.Is(err, types.ErrMFAUnavailable) {
		t.Fatalf("expected verify totp to return mfa unavailable, got %v", err)
	}
}
