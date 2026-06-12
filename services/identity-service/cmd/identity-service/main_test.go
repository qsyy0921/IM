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

func TestRotateRS256KeyRingPromotesCurrentToPublicOldKey(t *testing.T) {
	currentKey := generateGatewayTestRSAKey(t)
	oldKey := generateGatewayTestRSAKey(t)
	config := gatewayTokenRS256KeyRingConfig{
		Issuer: "issuer-rotate",
		Current: gatewayTokenRS256CurrentKey{
			KeyID:         "current-old",
			PrivateKeyPEM: testGatewayRSAPrivateKeyPEM(currentKey),
		},
		OldPublicKeys: []tokeninfra.JWK{testGatewayRSAJWK(t, oldKey, "previous-old")},
	}

	rotated, err := rotateRS256KeyRing(config, gatewayTokenKeyRingRotateOptions{
		NewKeyID:    "current-new",
		RSABits:     2048,
		OldKeyLimit: 2,
		Now:         time.Unix(1_800_000_000, 0),
	})
	if err != nil {
		t.Fatalf("rotate keyring: %v", err)
	}
	if rotated.Issuer != "issuer-rotate" {
		t.Fatalf("expected issuer to be preserved, got %q", rotated.Issuer)
	}
	if rotated.Current.KeyID != "current-new" || rotated.Current.PrivateKeyPEM == "" || rotated.Current.PrivateKeyFile != "" {
		t.Fatalf("unexpected current key after rotation: %+v", rotated.Current)
	}
	if _, err := tokeninfra.NewRS256SignerFromPEM(rotated.Current.PrivateKeyPEM, rotated.Current.KeyID, rotated.Issuer); err != nil {
		t.Fatalf("rotated current key should be signable: %v", err)
	}
	if len(rotated.OldPublicKeys) != 2 {
		t.Fatalf("expected two old public keys, got %+v", rotated.OldPublicKeys)
	}
	if rotated.OldPublicKeys[0].KeyID != "current-old" || rotated.OldPublicKeys[0].Modulus != testGatewayRSAJWK(t, currentKey, "current-old").Modulus {
		t.Fatalf("expected previous current key first in old public keys, got %+v", rotated.OldPublicKeys[0])
	}
	if rotated.OldPublicKeys[1].KeyID != "previous-old" {
		t.Fatalf("expected previous old key to remain, got %+v", rotated.OldPublicKeys[1])
	}
	for _, key := range rotated.OldPublicKeys {
		if key.Key != "" || key.PrivateExponent != "" || key.Prime1 != "" || key.Prime2 != "" || key.Exponent1 != "" || key.Exponent2 != "" || key.Coefficient != "" || len(key.OtherPrimes) != 0 {
			t.Fatalf("old public keys must not contain private material: %+v", key)
		}
	}
}

func TestRotateRS256KeyRingDefaultKidAndOldKeyLimit(t *testing.T) {
	currentKey := generateGatewayTestRSAKey(t)
	old1 := generateGatewayTestRSAKey(t)
	old2 := generateGatewayTestRSAKey(t)
	config := gatewayTokenRS256KeyRingConfig{
		Current: gatewayTokenRS256CurrentKey{
			KeyID:         "current-old",
			PrivateKeyPEM: testGatewayRSAPrivateKeyPEM(currentKey),
		},
		OldPublicKeys: []tokeninfra.JWK{
			testGatewayRSAJWK(t, old1, "old-1"),
			testGatewayRSAJWK(t, old2, "old-2"),
		},
	}

	rotated, err := rotateRS256KeyRing(config, gatewayTokenKeyRingRotateOptions{
		RSABits:     2048,
		OldKeyLimit: 1,
		Now:         time.Date(2026, 6, 13, 4, 5, 6, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("rotate keyring: %v", err)
	}
	if rotated.Current.KeyID != "nexusim-gateway-rs256-20260613T040506Z" {
		t.Fatalf("unexpected generated kid: %s", rotated.Current.KeyID)
	}
	if len(rotated.OldPublicKeys) != 1 || rotated.OldPublicKeys[0].KeyID != "current-old" {
		t.Fatalf("expected old key limit to keep only previous current key, got %+v", rotated.OldPublicKeys)
	}
}

func TestRotateRS256KeyRingRejectsDuplicateNewKid(t *testing.T) {
	currentKey := generateGatewayTestRSAKey(t)
	oldKey := generateGatewayTestRSAKey(t)
	config := gatewayTokenRS256KeyRingConfig{
		Current: gatewayTokenRS256CurrentKey{
			KeyID:         "current-old",
			PrivateKeyPEM: testGatewayRSAPrivateKeyPEM(currentKey),
		},
		OldPublicKeys: []tokeninfra.JWK{testGatewayRSAJWK(t, oldKey, "old-1")},
	}

	if _, err := rotateRS256KeyRing(config, gatewayTokenKeyRingRotateOptions{NewKeyID: "current-old", RSABits: 2048, OldKeyLimit: 2}); err == nil {
		t.Fatal("expected duplicate current kid to fail")
	}
	if _, err := rotateRS256KeyRing(config, gatewayTokenKeyRingRotateOptions{NewKeyID: "old-1", RSABits: 2048, OldKeyLimit: 2}); err == nil {
		t.Fatal("expected duplicate old public kid to fail")
	}
}

func TestRotateRS256KeyRingFileWritesUpdatedKeyRing(t *testing.T) {
	currentKey := generateGatewayTestRSAKey(t)
	path := filepath.Join(t.TempDir(), "gateway-keyring.json")
	config := gatewayTokenRS256KeyRingConfig{
		Current: gatewayTokenRS256CurrentKey{
			KeyID:         "current-old",
			PrivateKeyPEM: testGatewayRSAPrivateKeyPEM(currentKey),
		},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal keyring: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write keyring: %v", err)
	}

	rotated, err := rotateRS256KeyRingFile(path, gatewayTokenKeyRingRotateOptions{
		NewKeyID:    "current-new",
		RSABits:     2048,
		OldKeyLimit: 3,
	})
	if err != nil {
		t.Fatalf("rotate keyring file: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rotated keyring: %v", err)
	}
	var onDisk gatewayTokenRS256KeyRingConfig
	if err := json.Unmarshal(content, &onDisk); err != nil {
		t.Fatalf("unmarshal rotated keyring: %v", err)
	}
	if onDisk.Current.KeyID != rotated.Current.KeyID || len(onDisk.OldPublicKeys) != 1 || onDisk.OldPublicKeys[0].KeyID != "current-old" {
		t.Fatalf("unexpected keyring on disk: %+v", onDisk)
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

func TestMFASecretManagerUsesConfiguredKeyRing(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_MFA_SECRET_KEYRING_JSON", `{"current":"v2","keys":{"local-v1":"old-mfa-key","v2":"new-mfa-key"}}`)
	t.Setenv("NEXUSIM_IDENTITY_MFA_SECRET_KEYRING_FILE", "")
	t.Setenv("NEXUSIM_IDENTITY_MFA_SECRET_KEY", "")
	manager, err := newMFASecretManager()
	if err != nil {
		t.Fatalf("new mfa manager: %v", err)
	}
	_, encrypted, err := manager.NewTOTPSecret()
	if err != nil {
		t.Fatalf("new totp secret: %v", err)
	}
	if encrypted.KeyVersion != "v2" {
		t.Fatalf("expected current key version v2, got %q", encrypted.KeyVersion)
	}
}

func TestLoadSecretKeyRingConfigReadsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret-keyring.json")
	if err := os.WriteFile(path, []byte(`{"current":"v2","keys":{"local-v1":"old","v2":"new"}}`), 0o600); err != nil {
		t.Fatalf("write keyring file: %v", err)
	}
	t.Setenv("TEST_SECRET_KEYRING_JSON", "")
	t.Setenv("TEST_SECRET_KEYRING_FILE", path)
	config, ok, err := loadSecretKeyRingConfig("TEST_SECRET_KEYRING_JSON", "TEST_SECRET_KEYRING_FILE")
	if err != nil {
		t.Fatalf("load secret keyring: %v", err)
	}
	if !ok || config.Current != "v2" || config.Keys["local-v1"] != "old" || config.Keys["v2"] != "new" {
		t.Fatalf("unexpected keyring config ok=%t config=%+v", ok, config)
	}
}

func TestLoadSecretKeyRingConfigRejectsMissingCurrent(t *testing.T) {
	t.Setenv("TEST_SECRET_KEYRING_JSON", `{"keys":{"v1":"secret"}}`)
	t.Setenv("TEST_SECRET_KEYRING_FILE", "")
	if _, ok, err := loadSecretKeyRingConfig("TEST_SECRET_KEYRING_JSON", "TEST_SECRET_KEYRING_FILE"); err == nil || !ok {
		t.Fatalf("expected configured keyring with missing current to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadSecretKeyRingConfigRejectsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty-keyring.json")
	if err := os.WriteFile(path, []byte(" \n"), 0o600); err != nil {
		t.Fatalf("write empty keyring: %v", err)
	}
	t.Setenv("TEST_SECRET_KEYRING_JSON", "")
	t.Setenv("TEST_SECRET_KEYRING_FILE", path)
	if _, ok, err := loadSecretKeyRingConfig("TEST_SECRET_KEYRING_JSON", "TEST_SECRET_KEYRING_FILE"); err == nil || !ok {
		t.Fatalf("expected configured empty keyring file to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadSecretKeyRingConfigRejectsMalformedJSON(t *testing.T) {
	t.Setenv("TEST_SECRET_KEYRING_JSON", "{")
	t.Setenv("TEST_SECRET_KEYRING_FILE", "")
	if _, ok, err := loadSecretKeyRingConfig("TEST_SECRET_KEYRING_JSON", "TEST_SECRET_KEYRING_FILE"); err == nil || !ok {
		t.Fatalf("expected malformed keyring json to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadSecretKeyRingConfigRejectsCurrentNotInKeys(t *testing.T) {
	t.Setenv("TEST_SECRET_KEYRING_JSON", `{"current":"v2","keys":{"v1":"secret"}}`)
	t.Setenv("TEST_SECRET_KEYRING_FILE", "")
	if _, ok, err := loadSecretKeyRingConfig("TEST_SECRET_KEYRING_JSON", "TEST_SECRET_KEYRING_FILE"); err == nil || !ok {
		t.Fatalf("expected keyring missing current key to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadSecretKeyRingConfigRejectsTrimmedDuplicateKeyVersion(t *testing.T) {
	t.Setenv("TEST_SECRET_KEYRING_JSON", `{"current":"v1","keys":{"v1":"secret-1"," v1 ":"secret-2"}}`)
	t.Setenv("TEST_SECRET_KEYRING_FILE", "")
	if _, ok, err := loadSecretKeyRingConfig("TEST_SECRET_KEYRING_JSON", "TEST_SECRET_KEYRING_FILE"); err == nil || !ok {
		t.Fatalf("expected duplicate key version to fail, ok=%t err=%v", ok, err)
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

func TestChallengeDeliveryTokenManagerUsesConfiguredKeyRing(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEYRING_JSON", `{"current":"v2","keys":{"local-v1":"old-delivery-key","v2":"new-delivery-key"}}`)
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEYRING_FILE", "")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEY", "")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_SECRET", "")
	manager, err := newChallengeDeliveryTokenManager()
	if err != nil {
		t.Fatalf("new challenge delivery token manager: %v", err)
	}
	encrypted, err := manager.SealChallengeToken("challenge-token")
	if err != nil {
		t.Fatalf("seal challenge token: %v", err)
	}
	if encrypted.KeyVersion != "v2" {
		t.Fatalf("expected current key version v2, got %q", encrypted.KeyVersion)
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

func TestChallengeRequestLimitCleanupConfigFromEnv(t *testing.T) {
	config, err := challengeRequestLimitCleanupConfigFromEnv()
	if err != nil {
		t.Fatalf("default cleanup config: %v", err)
	}
	if config.Retention != 24*time.Hour || config.BatchSize != 5000 {
		t.Fatalf("unexpected default cleanup config: %+v", config)
	}

	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_RETENTION", "2h")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_CLEANUP_BATCH_SIZE", "123")
	config, err = challengeRequestLimitCleanupConfigFromEnv()
	if err != nil {
		t.Fatalf("custom cleanup config: %v", err)
	}
	if config.Retention != 2*time.Hour || config.BatchSize != 123 {
		t.Fatalf("unexpected custom cleanup config: %+v", config)
	}
}

func TestChallengeRequestLimitCleanupConfigRejectsInvalidValues(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_RETENTION", "0")
	if _, err := challengeRequestLimitCleanupConfigFromEnv(); err == nil {
		t.Fatal("expected zero retention to fail")
	}

	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_RETENTION", "1h")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_CLEANUP_BATCH_SIZE", "0")
	if _, err := challengeRequestLimitCleanupConfigFromEnv(); err == nil {
		t.Fatal("expected zero batch size to fail")
	}
}
