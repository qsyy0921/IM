package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
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
	base := tokeninfra.JWKSet{Keys: []tokeninfra.JWK{testGatewayRSAJWK(t, generateGatewayTestRSAKey(t), "current")}}

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

func TestLoadRS256KeyRingSignerSignsWithCurrentAndPublishesOldPublicKeys(t *testing.T) {
	currentKey := generateGatewayTestRSAKey(t)
	oldKey := generateGatewayTestRSAKey(t)
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "current.pem")
	if err := os.WriteFile(currentPath, []byte(testGatewayRSAPrivateKeyPEM(currentKey)), 0o600); err != nil {
		t.Fatalf("write current key: %v", err)
	}
	config := gatewayTokenRS256KeyRingConfig{
		Issuer: "issuer-keyring",
		Current: gatewayTokenRS256CurrentKey{
			KeyID:          "current-kid",
			PrivateKeyFile: currentPath,
		},
		OldPublicKeys: []tokeninfra.JWK{
			testGatewayRSAJWK(t, oldKey, "old-kid"),
			testGatewayRSAJWK(t, oldKey, "current-kid"),
		},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal keyring: %v", err)
	}
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING_JSON", string(raw))
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING_FILE", "")

	signer, ok, err := loadRS256KeyRingSigner()
	if err != nil {
		t.Fatalf("load keyring signer: %v", err)
	}
	if !ok {
		t.Fatal("expected keyring signer to be configured")
	}
	token, err := signer.SignGatewayToken(types.TokenClaims{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		DeviceID:  "device-1",
		Audience:  "push-gateway",
		IssuedAt:  time.Unix(1_799_999_900, 0).Unix(),
		ExpiresAt: time.Unix(1_800_000_000, 0).Unix(),
	})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected jwt: %s", token)
	}
	var header map[string]any
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	if err := json.Unmarshal(headerRaw, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header["alg"] != "RS256" || header["kid"] != "current-kid" {
		t.Fatalf("unexpected header: %+v", header)
	}
	var claims map[string]any
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if claims["iss"] != "issuer-keyring" {
		t.Fatalf("expected keyring issuer, got %+v", claims)
	}

	jwks := signer.JWKSet()
	if len(jwks.Keys) != 2 {
		t.Fatalf("expected current and old public keys, got %+v", jwks.Keys)
	}
	if jwks.Keys[0].KeyID != "current-kid" || jwks.Keys[1].KeyID != "old-kid" {
		t.Fatalf("unexpected key order: %+v", jwks.Keys)
	}
	if jwks.Keys[0].Modulus != testGatewayRSAJWK(t, currentKey, "current-kid").Modulus {
		t.Fatalf("expected current key to win duplicate old kid, got %+v", jwks.Keys[0])
	}
	for _, key := range jwks.Keys {
		if key.Key != "" {
			t.Fatalf("jwks must not expose symmetric/private key material: %+v", key)
		}
	}
}

func TestLoadRS256KeyRingSignerRejectsSymmetricOldPublicKey(t *testing.T) {
	currentKey := generateGatewayTestRSAKey(t)
	config := gatewayTokenRS256KeyRingConfig{
		Current: gatewayTokenRS256CurrentKey{
			KeyID:         "current-kid",
			PrivateKeyPEM: testGatewayRSAPrivateKeyPEM(currentKey),
		},
		OldPublicKeys: []tokeninfra.JWK{{
			KeyType:   "oct",
			KeyUse:    "sig",
			KeyID:     "shared",
			Algorithm: "HS256",
			Key:       "secret",
		}},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal keyring: %v", err)
	}
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING_JSON", string(raw))
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING_FILE", "")

	if _, ok, err := loadRS256KeyRingSigner(); err == nil || !ok {
		t.Fatalf("expected configured keyring with symmetric old key to fail, ok=%v err=%v", ok, err)
	}
}

func TestLoadRS256KeyRingSignerRejectsPrivateOldPublicKeyMaterial(t *testing.T) {
	currentKey := generateGatewayTestRSAKey(t)
	oldKey := testGatewayRSAJWK(t, generateGatewayTestRSAKey(t), "old-kid")
	oldKey.PrivateExponent = "private"
	config := gatewayTokenRS256KeyRingConfig{
		Current: gatewayTokenRS256CurrentKey{
			KeyID:         "current-kid",
			PrivateKeyPEM: testGatewayRSAPrivateKeyPEM(currentKey),
		},
		OldPublicKeys: []tokeninfra.JWK{oldKey},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal keyring: %v", err)
	}
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING_JSON", string(raw))
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING_FILE", "")

	if _, ok, err := loadRS256KeyRingSigner(); err == nil || !ok {
		t.Fatalf("expected configured keyring with private jwk material to fail, ok=%v err=%v", ok, err)
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

func testGatewayRSAPrivateKeyPEM(privateKey *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
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

func TestChallengeNotifierAcceptsOutboxMode(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_MODE", "outbox")
	notifier, mode, err := newChallengeNotifier()
	if err != nil {
		t.Fatalf("new outbox notifier: %v", err)
	}
	if notifier == nil || mode != "outbox" {
		t.Fatalf("expected noop notifier for outbox mode, mode=%q notifier=%T", mode, notifier)
	}
}

func TestChallengeDeliveryTokenManagerRequiresDedicatedKey(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEY", "")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_SECRET", "")
	_, err := newChallengeDeliveryTokenManager()
	if !errors.Is(err, types.ErrChallengeDeliveryFailed) {
		t.Fatalf("expected missing challenge delivery key to fail, got %v", err)
	}

	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEY", "challenge-delivery-key")
	manager, err := newChallengeDeliveryTokenManager()
	if err != nil {
		t.Fatalf("new challenge delivery token manager: %v", err)
	}
	if manager == nil {
		t.Fatal("expected challenge delivery token manager")
	}
}
