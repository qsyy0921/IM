package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	tokeninfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/token"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestGatewayTokenJWKSetWithAdditionalKeysMergesAndDeduplicates(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_JSON", `{"keys":[{"kty":"RSA","use":"sig","kid":"old","alg":"RS256","n":"abc","e":"AQAB"},{"kty":"RSA","use":"sig","kid":"current","alg":"RS256","n":"duplicate","e":"AQAB"}]}`)
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_FILE", "")
	base := tokeninfra.JWKSet{Keys: []tokeninfra.JWK{{
		KeyType:   "RSA",
		KeyUse:    "sig",
		KeyID:     "current",
		Algorithm: "RS256",
		Modulus:   "base",
		Exponent:  "AQAB",
	}}}

	merged, err := gatewayTokenJWKSetWithAdditionalKeys(base)
	if err != nil {
		t.Fatalf("merge jwks: %v", err)
	}
	if len(merged.Keys) != 2 {
		t.Fatalf("expected current plus one old key, got %+v", merged.Keys)
	}
	if merged.Keys[0].KeyID != "current" || merged.Keys[0].Modulus != "base" {
		t.Fatalf("expected base current key to stay first and win duplicates, got %+v", merged.Keys[0])
	}
	if merged.Keys[1].KeyID != "old" {
		t.Fatalf("expected old key to be appended, got %+v", merged.Keys[1])
	}
}

func TestLoadAdditionalGatewayTokenJWKSetReadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jwks.json")
	if err := os.WriteFile(path, []byte(`{"keys":[{"kty":"RSA","use":"sig","kid":"old","alg":"RS256","n":"abc","e":"AQAB"}]}`), 0o600); err != nil {
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

func TestDisabledMFASecretManagerReturnsMFAUnavailable(t *testing.T) {
	manager := disabledMFASecretManager{}
	if _, _, err := manager.NewTOTPSecret(); !errors.Is(err, types.ErrMFAUnavailable) {
		t.Fatalf("expected new totp to return mfa unavailable, got %v", err)
	}
	if _, err := manager.VerifyTOTP(types.EncryptedMFASecret{}, "123456", time.Now()); !errors.Is(err, types.ErrMFAUnavailable) {
		t.Fatalf("expected verify totp to return mfa unavailable, got %v", err)
	}
}
