package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/app"
	notificationinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/notification"
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

func TestIdentityOIDCDiscoveryDefaultsToDisabled(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_OIDC_DISCOVERY_ENABLED", "")
	signer, err := tokeninfra.NewJWTSigner("secret", "kid", "https://identity.nexusim.test")
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	discovery, err := identityOIDCDiscoveryFromEnv(signer, tokeninfra.JWKSet{})
	if err != nil {
		t.Fatalf("discovery disabled should not fail: %v", err)
	}
	if discovery != nil {
		t.Fatalf("expected nil discovery when disabled, got %+v", discovery)
	}
}

func TestIdentityOIDCDiscoveryRequiresPublicJWKS(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_OIDC_DISCOVERY_ENABLED", "true")
	t.Setenv("NEXUSIM_IDENTITY_OIDC_ISSUER", "https://identity.nexusim.test")
	signer, err := tokeninfra.NewJWTSigner("secret", "kid", "https://identity.nexusim.test")
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	if _, err := identityOIDCDiscoveryFromEnv(signer, signer.JWKSet()); err == nil {
		t.Fatal("expected OIDC discovery without public JWKS to fail")
	}
}

func TestIdentityOIDCDiscoveryRejectsNonURLIssuer(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_OIDC_DISCOVERY_ENABLED", "true")
	signer, err := tokeninfra.NewJWTSigner("secret", "kid", "nexusim-identity")
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	jwkSet := tokeninfra.JWKSet{Keys: []tokeninfra.JWK{testGatewayRSAJWK(t, generateGatewayTestRSAKey(t), "current")}}
	if _, err := identityOIDCDiscoveryFromEnv(signer, jwkSet); err == nil {
		t.Fatal("expected non-url issuer to fail")
	}
}

func TestIdentityOIDCDiscoveryBuildsMetadata(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_OIDC_DISCOVERY_ENABLED", "true")
	t.Setenv("NEXUSIM_IDENTITY_OIDC_ISSUER", "")
	t.Setenv("NEXUSIM_IDENTITY_OIDC_JWKS_URI", "")
	privateKey := generateGatewayTestRSAKey(t)
	signer, err := tokeninfra.NewRS256SignerFromPEM(testGatewayRSAPrivateKeyPEM(privateKey), "current", "https://identity.nexusim.test")
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	discovery, err := identityOIDCDiscoveryFromEnv(signer, signer.JWKSet())
	if err != nil {
		t.Fatalf("build oidc discovery: %v", err)
	}
	if discovery == nil ||
		discovery.Issuer != "https://identity.nexusim.test" ||
		discovery.JWKSURI != "https://identity.nexusim.test/.well-known/jwks.json" ||
		len(discovery.IDTokenSigningAlgValuesSupported) != 1 ||
		discovery.IDTokenSigningAlgValuesSupported[0] != "RS256" {
		t.Fatalf("unexpected oidc discovery: %+v", discovery)
	}
}

func TestIdentityProductionKeyGuardDefaultsToDisabled(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_PRODUCTION_KEY_GUARD", "")
	if err := validateIdentityProductionKeyGuardFromEnv(identityRuntimeKeyGuardScope{
		GatewayToken:           true,
		MFA:                    true,
		MFARecovery:            true,
		ChallengeRequestLimit:  true,
		ChallengeDeliveryToken: true,
	}); err != nil {
		t.Fatalf("disabled key guard should not fail: %v", err)
	}
}

func TestIdentityProductionKeyGuardRejectsNonProductionKeyModes(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_PRODUCTION_KEY_GUARD", "true")
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT", "legacy")

	err := validateIdentityProductionKeyGuardFromEnv(identityRuntimeKeyGuardScope{
		GatewayToken:           true,
		MFA:                    true,
		MFARecovery:            true,
		ChallengeRequestLimit:  true,
		ChallengeDeliveryToken: true,
	})
	if err == nil {
		t.Fatal("expected production key guard to reject non-production key modes")
	}
	message := err.Error()
	for _, want := range []string{
		"NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT",
		"NEXUSIM_IDENTITY_MFA_SECRET_KEY",
		"NEXUSIM_IDENTITY_MFA_RECOVERY_CODE_SECRET",
		"NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_SECRET",
		"NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEY",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected error to mention %s, got %q", want, message)
		}
	}
}

func TestIdentityProductionKeyGuardAcceptsExplicitDedicatedKeys(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_PRODUCTION_KEY_GUARD", "true")
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT", "rs256")
	t.Setenv("NEXUSIM_IDENTITY_MFA_SECRET_KEYRING_JSON", `{"current":"v2","keys":{"v1":"old-mfa-key","v2":"new-mfa-key"}}`)
	t.Setenv("NEXUSIM_IDENTITY_MFA_SECRET_KEYRING_FILE", "")
	t.Setenv("NEXUSIM_IDENTITY_MFA_RECOVERY_CODE_SECRET", "recovery-key")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_SECRET", "request-limit-key")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEY", "challenge-token-key")

	if err := validateIdentityProductionKeyGuardFromEnv(identityRuntimeKeyGuardScope{
		GatewayToken:           true,
		MFA:                    true,
		MFARecovery:            true,
		ChallengeRequestLimit:  true,
		ChallengeDeliveryToken: true,
	}); err != nil {
		t.Fatalf("expected explicit dedicated keys to pass: %v", err)
	}
}

func TestIdentityProductionKeyGuardWorkerScopeDoesNotRequireGatewayKeys(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_PRODUCTION_KEY_GUARD", "true")
	t.Setenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT", "legacy")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEY", "challenge-token-key")

	if err := validateIdentityProductionKeyGuardFromEnv(identityRuntimeKeyGuardScope{
		ChallengeDeliveryToken: true,
	}); err != nil {
		t.Fatalf("expected worker token-only scope to pass: %v", err)
	}
}

func TestIdentityMFARecoveryRiskPolicyUsesDedicatedDefaults(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_MFA_MAX_FAILED_ATTEMPTS", "7")
	t.Setenv("NEXUSIM_IDENTITY_MFA_FAILURE_WINDOW", "21m")
	t.Setenv("NEXUSIM_IDENTITY_MFA_LOCK_DURATION", "22m")
	t.Setenv("NEXUSIM_IDENTITY_MFA_RECOVERY_MAX_FAILED_ATTEMPTS", "")
	t.Setenv("NEXUSIM_IDENTITY_MFA_RECOVERY_FAILURE_WINDOW", "")
	t.Setenv("NEXUSIM_IDENTITY_MFA_RECOVERY_LOCK_DURATION", "")

	mfa := identityMFARiskPolicyFromEnv()
	recovery := identityMFARecoveryRiskPolicyFromEnv()

	if recovery.MaxFailedAttempts != app.DefaultMFAMaxFailedAttempts ||
		recovery.FailureWindow != app.DefaultMFAFailureWindow ||
		recovery.LockDuration != app.DefaultMFALockDuration {
		t.Fatalf("expected recovery risk to use dedicated defaults, got %+v", recovery)
	}
	if mfa.MaxFailedAttempts != 7 || mfa.FailureWindow != 21*time.Minute || mfa.LockDuration != 22*time.Minute {
		t.Fatalf("expected MFA risk policy to remain independently configurable, got %+v", mfa)
	}
}

func TestIdentityMFARecoveryRiskPolicyOverridesMFAEnv(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_MFA_MAX_FAILED_ATTEMPTS", "7")
	t.Setenv("NEXUSIM_IDENTITY_MFA_FAILURE_WINDOW", "21m")
	t.Setenv("NEXUSIM_IDENTITY_MFA_LOCK_DURATION", "22m")
	t.Setenv("NEXUSIM_IDENTITY_MFA_RECOVERY_MAX_FAILED_ATTEMPTS", "2")
	t.Setenv("NEXUSIM_IDENTITY_MFA_RECOVERY_FAILURE_WINDOW", "3m")
	t.Setenv("NEXUSIM_IDENTITY_MFA_RECOVERY_LOCK_DURATION", "4m")

	mfa := identityMFARiskPolicyFromEnv()
	recovery := identityMFARecoveryRiskPolicyFromEnv()

	if recovery.MaxFailedAttempts != 2 || recovery.FailureWindow != 3*time.Minute || recovery.LockDuration != 4*time.Minute {
		t.Fatalf("expected recovery risk to use dedicated env policy, got %+v", recovery)
	}
	if mfa.MaxFailedAttempts != 7 || mfa.FailureWindow != 21*time.Minute || mfa.LockDuration != 22*time.Minute {
		t.Fatalf("expected MFA risk policy to remain independent, got %+v", mfa)
	}
}

func TestIdentityTraceConfigDefaultsToDisabled(t *testing.T) {
	clearIdentityTraceConfig(t)
	config, err := identityTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load identity trace config: %v", err)
	}
	if config.Enabled ||
		config.ServiceName != "identity-service" ||
		config.Exporter != "stdout" ||
		config.SamplingRatio != 1 {
		t.Fatalf("unexpected default trace config: %+v", config)
	}
}

func TestIdentityTraceConfigLoadsOTLPGRPC(t *testing.T) {
	clearIdentityTraceConfig(t)
	t.Setenv("NEXUSIM_IDENTITY_OTEL_TRACES_ENABLED", "true")
	t.Setenv("NEXUSIM_IDENTITY_OTEL_SERVICE_NAME", "identity-service-test")
	t.Setenv("NEXUSIM_IDENTITY_OTEL_TRACES_EXPORTER", "otlp-grpc")
	t.Setenv("NEXUSIM_IDENTITY_OTEL_TRACES_OTLP_ENDPOINT", "127.0.0.1:4317")
	t.Setenv("NEXUSIM_IDENTITY_OTEL_TRACES_OTLP_INSECURE", "true")
	t.Setenv("NEXUSIM_IDENTITY_OTEL_TRACES_SAMPLING_RATIO", "0.5")

	config, err := identityTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load identity trace config: %v", err)
	}
	if !config.Enabled ||
		config.ServiceName != "identity-service-test" ||
		config.Exporter != "otlp-grpc" ||
		config.OTLPEndpoint != "127.0.0.1:4317" ||
		!config.OTLPInsecure ||
		config.SamplingRatio != 0.5 {
		t.Fatalf("unexpected otlp trace config: %+v", config)
	}
}

func TestIdentityTraceConfigRejectsInvalidValues(t *testing.T) {
	clearIdentityTraceConfig(t)
	t.Setenv("NEXUSIM_IDENTITY_OTEL_TRACES_ENABLED", "sometimes")
	if _, err := identityTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid enabled bool to fail")
	}

	clearIdentityTraceConfig(t)
	t.Setenv("NEXUSIM_IDENTITY_OTEL_TRACES_SAMPLING_RATIO", "2")
	if _, err := identityTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid sampling ratio to fail")
	}

	clearIdentityTraceConfig(t)
	t.Setenv("NEXUSIM_IDENTITY_OTEL_TRACES_OTLP_INSECURE", "sometimes")
	if _, err := identityTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid otlp insecure bool to fail")
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

func clearIdentityGRPCTLSConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE", "")
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_KEY_FILE", "")
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_CA_FILE", "")
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_REQUIRE_CLIENT_CERT", "")
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "")
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_ALLOWED_URIS", "")
}

func clearIdentityTraceConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_IDENTITY_OTEL_TRACES_ENABLED", "")
	t.Setenv("NEXUSIM_IDENTITY_OTEL_SERVICE_NAME", "")
	t.Setenv("NEXUSIM_IDENTITY_OTEL_TRACES_EXPORTER", "")
	t.Setenv("NEXUSIM_IDENTITY_OTEL_TRACES_OTLP_ENDPOINT", "")
	t.Setenv("NEXUSIM_IDENTITY_OTEL_TRACES_OTLP_INSECURE", "")
	t.Setenv("NEXUSIM_IDENTITY_OTEL_TRACES_SAMPLING_RATIO", "")
}

func writeIdentityTLSTestCert(t *testing.T, dir string, name string) (string, string) {
	return writeIdentityTLSTestCertWithSANs(t, dir, name, []string{"localhost"}, nil)
}

func writeIdentityTLSTestCertWithSANs(t *testing.T, dir string, name string, dnsNames []string, uriNames []string) (string, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate tls key: %v", err)
	}
	serialNumber, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		t.Fatalf("generate tls serial: %v", err)
	}
	uris := make([]*url.URL, 0, len(uriNames))
	for _, uriName := range uriNames {
		parsed, err := url.Parse(uriName)
		if err != nil {
			t.Fatalf("parse tls uri san: %v", err)
		}
		uris = append(uris, parsed)
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "identity-" + name,
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              dnsNames,
		URIs:                  uris,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create tls cert: %v", err)
	}
	certFile := filepath.Join(dir, name+".crt")
	keyFile := filepath.Join(dir, name+".key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write tls cert: %v", err)
	}
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKeyBytes}), 0o600); err != nil {
		t.Fatalf("write tls key: %v", err)
	}
	return certFile, keyFile
}

func readIdentityTLSTestCert(t *testing.T, certFile string) *x509.Certificate {
	t.Helper()
	raw, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("read tls cert: %v", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		t.Fatalf("expected PEM certificate in %s", certFile)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse tls cert: %v", err)
	}
	return cert
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

func TestLoadIdentityGRPCCredentialsFromEnvDisabledByDefault(t *testing.T) {
	clearIdentityGRPCTLSConfig(t)
	creds, ok, err := loadIdentityGRPCCredentialsFromEnv()
	if err != nil {
		t.Fatalf("load grpc credentials: %v", err)
	}
	if ok || creds != nil {
		t.Fatalf("expected grpc tls to be disabled by default, ok=%t creds=%T", ok, creds)
	}
}

func TestValidateTrustedMetadataListenerConfigAllowsPrivateAddressWithoutMTLS(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"172.31.50.10:10600",
		"metadata",
		nil,
	)
	if err != nil {
		t.Fatalf("expected private address to be allowed without mTLS, got %v", err)
	}
}

func TestValidateTrustedMetadataListenerConfigRequiresMTLSForPublicAddress(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10600",
		"verified-metadata",
		nil,
	)
	if err == nil {
		t.Fatalf("expected public address without mTLS client cert to fail")
	}
}

func TestValidateTrustedMetadataListenerConfigAllowsMTLSForPublicAddress(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10600",
		"verified-metadata",
		&tls.Config{ClientAuth: tls.RequireAndVerifyClientCert},
	)
	if err != nil {
		t.Fatalf("expected public address with mTLS client cert to be allowed, got %v", err)
	}
}

func TestValidateTrustedMetadataListenerConfigIgnoresBodyAuth(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10600",
		"body",
		nil,
	)
	if err != nil {
		t.Fatalf("expected body auth to skip trusted metadata guard, got %v", err)
	}
}

func TestValidateIdentityDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	for _, addr := range []string{"", "127.0.0.1:11905", "localhost:11905", "172.31.50.10:11905"} {
		if err := validateIdentityDebugListenerConfig(addr, false); err != nil {
			t.Fatalf("expected identity debug listener %q to be allowed: %v", addr, err)
		}
	}
}

func TestValidateIdentityDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:11905", ":11905", "8.8.8.8:11905"} {
		if err := validateIdentityDebugListenerConfig(addr, false); err == nil {
			t.Fatalf("expected identity debug listener %q to be rejected by default", addr)
		}
	}
}

func TestValidateIdentityDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateIdentityDebugListenerConfig("0.0.0.0:11905", true); err != nil {
		t.Fatalf("expected explicit public identity debug listener opt-in to be allowed: %v", err)
	}
}

func TestLoadIdentityGRPCCredentialsFromEnvRequiresCertKeyPair(t *testing.T) {
	clearIdentityGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE", "server.crt")
	if _, ok, err := loadIdentityGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected partial grpc tls config to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadIdentityGRPCCredentialsFromEnvLoadsServerTLS(t *testing.T) {
	clearIdentityGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeIdentityTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_KEY_FILE", keyFile)
	tlsConfig, ok, err := identityGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load grpc tls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected grpc tls config, ok=%t", ok)
	}
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected TLS 1.2 minimum, got %d", tlsConfig.MinVersion)
	}

	creds, ok, err := loadIdentityGRPCCredentialsFromEnv()
	if err != nil {
		t.Fatalf("load grpc tls credentials: %v", err)
	}
	if !ok || creds == nil {
		t.Fatalf("expected grpc tls credentials, ok=%t creds=%T", ok, creds)
	}
}

func TestLoadIdentityGRPCCredentialsFromEnvRejectsInvalidRequireClientCert(t *testing.T) {
	clearIdentityGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_REQUIRE_CLIENT_CERT", "sometimes")
	if _, ok, err := loadIdentityGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected invalid client-cert bool to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadIdentityGRPCCredentialsFromEnvRequiresClientCAForMTLS(t *testing.T) {
	clearIdentityGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeIdentityTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_REQUIRE_CLIENT_CERT", "true")
	if _, ok, err := loadIdentityGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected mtls without ca to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadIdentityGRPCCredentialsFromEnvRequiresClientCAForClientAllowlist(t *testing.T) {
	clearIdentityGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeIdentityTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "push-gateway.nexusim.local")
	if _, ok, err := loadIdentityGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected client identity allowlist without ca to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadIdentityGRPCCredentialsFromEnvRejectsInvalidClientCAPEM(t *testing.T) {
	clearIdentityGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeIdentityTLSTestCert(t, dir, "server")
	caFile := filepath.Join(dir, "invalid-ca.pem")
	if err := os.WriteFile(caFile, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write invalid ca: %v", err)
	}
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_CA_FILE", caFile)
	if _, ok, err := loadIdentityGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected invalid ca pem to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadIdentityGRPCCredentialsFromEnvRejectsInvalidClientURIAllowlist(t *testing.T) {
	clearIdentityGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_ALLOWED_URIS", "://bad-uri")
	if _, ok, err := loadIdentityGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected invalid client uri allowlist to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadIdentityGRPCCredentialsFromEnvLoadsMTLS(t *testing.T) {
	clearIdentityGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeIdentityTLSTestCert(t, dir, "server")
	caFile, _ := writeIdentityTLSTestCert(t, dir, "ca")
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_CA_FILE", caFile)
	tlsConfig, ok, err := identityGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load grpc mtls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected grpc mtls config, ok=%t", ok)
	}
	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected client cert verification, got %v", tlsConfig.ClientAuth)
	}
	if tlsConfig.ClientCAs == nil {
		t.Fatalf("expected client CA pool")
	}

	creds, ok, err := loadIdentityGRPCCredentialsFromEnv()
	if err != nil {
		t.Fatalf("load grpc mtls credentials: %v", err)
	}
	if !ok || creds == nil {
		t.Fatalf("expected grpc mtls credentials, ok=%t creds=%T", ok, creds)
	}
}

func TestIdentityGRPCTLSConfigAllowsClientDNSName(t *testing.T) {
	clearIdentityGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeIdentityTLSTestCert(t, dir, "server")
	caFile, _ := writeIdentityTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeIdentityTLSTestCertWithSANs(t, dir, "client", []string{"push-gateway.nexusim.local"}, nil)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", " PUSH-GATEWAY.NEXUSIM.LOCAL ")
	tlsConfig, ok, err := identityGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readIdentityTLSTestCert(t, clientCertFile)}}); err != nil {
		t.Fatalf("expected client dns identity to be allowed: %v", err)
	}
}

func TestIdentityGRPCTLSConfigAllowsClientURI(t *testing.T) {
	clearIdentityGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeIdentityTLSTestCert(t, dir, "server")
	caFile, _ := writeIdentityTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeIdentityTLSTestCertWithSANs(t, dir, "client", nil, []string{"spiffe://nexusim/push-gateway"})
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_ALLOWED_URIS", "spiffe://nexusim/push-gateway")
	tlsConfig, ok, err := identityGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readIdentityTLSTestCert(t, clientCertFile)}}); err != nil {
		t.Fatalf("expected client uri identity to be allowed: %v", err)
	}
}

func TestIdentityGRPCTLSConfigRejectsUnlistedClientIdentity(t *testing.T) {
	clearIdentityGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeIdentityTLSTestCert(t, dir, "server")
	caFile, _ := writeIdentityTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeIdentityTLSTestCertWithSANs(t, dir, "client", []string{"contacts-service.nexusim.local"}, nil)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "push-gateway.nexusim.local")
	tlsConfig, ok, err := identityGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readIdentityTLSTestCert(t, clientCertFile)}}); err == nil {
		t.Fatalf("expected unlisted client identity to be rejected")
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

func TestChallengeNotifierAcceptsSMTPMode(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_MODE", "smtp")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_SMTP_ADDR", "smtp.example.com:587")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_SMTP_FROM", "NexusIM <no-reply@example.com>")
	notifier, mode, err := newChallengeNotifier()
	if err != nil {
		t.Fatalf("new smtp notifier: %v", err)
	}
	if mode != "smtp" {
		t.Fatalf("expected smtp mode, got %q", mode)
	}
	if _, ok := notifier.(*notificationinfra.SMTPChallengeNotifier); !ok {
		t.Fatalf("expected smtp notifier, got %T", notifier)
	}
}

func TestChallengeSMTPTemplatesFromEnv(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_SMTP_SUBJECT_TEMPLATE", "NexusIM {purpose}")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_SMTP_SUBJECT_TEMPLATE_EMAIL_VERIFICATION", "Verify {purpose}")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_SMTP_SUBJECT_TEMPLATE_PASSWORD_RESET", "Reset {purpose}")
	templates := challengeSMTPTemplatesFromEnv("NEXUSIM_IDENTITY_CHALLENGE_SMTP_SUBJECT_TEMPLATE")
	if templates[types.ChallengeType("")] != "NexusIM {purpose}" {
		t.Fatalf("unexpected default subject template %q", templates[types.ChallengeType("")])
	}
	if templates[types.ChallengeTypeEmailVerification] != "Verify {purpose}" {
		t.Fatalf("unexpected email verification template %q", templates[types.ChallengeTypeEmailVerification])
	}
	if templates[types.ChallengeTypePasswordReset] != "Reset {purpose}" {
		t.Fatalf("unexpected password reset template %q", templates[types.ChallengeTypePasswordReset])
	}
}

func TestChallengeDeliveryWorkerNotifierAcceptsSMTPProvider(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_WORKER_PROVIDER", "smtp")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_SMTP_ADDR", "smtp.example.com:587")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_SMTP_FROM", "no-reply@example.com")
	notifier, provider, err := newChallengeDeliveryWorkerNotifier()
	if err != nil {
		t.Fatalf("new smtp worker notifier: %v", err)
	}
	if provider != "smtp" {
		t.Fatalf("expected smtp provider, got %q", provider)
	}
	if _, ok := notifier.(*notificationinfra.SMTPChallengeNotifier); !ok {
		t.Fatalf("expected smtp notifier, got %T", notifier)
	}
}

func TestChallengeDeliveryWorkerNotifierRejectsUnsupportedProvider(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_WORKER_PROVIDER", "sms")
	if _, _, err := newChallengeDeliveryWorkerNotifier(); err == nil {
		t.Fatalf("expected unsupported worker provider to fail")
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
	if config.Retention != 24*time.Hour || config.BatchSize != 5000 || config.DryRun {
		t.Fatalf("unexpected default cleanup config: %+v", config)
	}

	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_RETENTION", "2h")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_CLEANUP_BATCH_SIZE", "123")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_CLEANUP_DRY_RUN", "true")
	config, err = challengeRequestLimitCleanupConfigFromEnv()
	if err != nil {
		t.Fatalf("custom cleanup config: %v", err)
	}
	if config.Retention != 2*time.Hour || config.BatchSize != 123 || !config.DryRun {
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

func TestChallengeDeliveryRepairCleanupConfigFromEnv(t *testing.T) {
	config, err := challengeDeliveryRepairCleanupConfigFromEnv()
	if err != nil {
		t.Fatalf("default repair cleanup config: %v", err)
	}
	if config.Retention != 7*24*time.Hour || config.BatchSize != 5000 || config.DryRun {
		t.Fatalf("unexpected default repair cleanup config: %+v", config)
	}

	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_RETENTION", "48h")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_BATCH_SIZE", "321")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_DRY_RUN", "true")
	config, err = challengeDeliveryRepairCleanupConfigFromEnv()
	if err != nil {
		t.Fatalf("custom repair cleanup config: %v", err)
	}
	if config.Retention != 48*time.Hour || config.BatchSize != 321 || !config.DryRun {
		t.Fatalf("unexpected custom repair cleanup config: %+v", config)
	}
}

func TestChallengeDeliveryRepairCleanupConfigRejectsInvalidValues(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_RETENTION", "0")
	if _, err := challengeDeliveryRepairCleanupConfigFromEnv(); err == nil {
		t.Fatal("expected zero retention to fail")
	}

	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_RETENTION", "24h")
	t.Setenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_BATCH_SIZE", "0")
	if _, err := challengeDeliveryRepairCleanupConfigFromEnv(); err == nil {
		t.Fatal("expected zero batch size to fail")
	}
}

func TestOptionalRFC3339TimeEnv(t *testing.T) {
	t.Setenv("NEXUSIM_IDENTITY_TEST_OPTIONAL_TIME", "")
	parsed, err := optionalRFC3339TimeEnv("NEXUSIM_IDENTITY_TEST_OPTIONAL_TIME")
	if err != nil || parsed != nil {
		t.Fatalf("expected empty optional time to be nil, parsed=%v err=%v", parsed, err)
	}

	t.Setenv("NEXUSIM_IDENTITY_TEST_OPTIONAL_TIME", "2026-06-17T09:20:00+08:00")
	parsed, err = optionalRFC3339TimeEnv("NEXUSIM_IDENTITY_TEST_OPTIONAL_TIME")
	if err != nil || parsed == nil || parsed.Format(time.RFC3339) != "2026-06-17T01:20:00Z" {
		t.Fatalf("expected UTC RFC3339 time, parsed=%v err=%v", parsed, err)
	}

	t.Setenv("NEXUSIM_IDENTITY_TEST_OPTIONAL_TIME", "2026-06-17")
	if _, err := optionalRFC3339TimeEnv("NEXUSIM_IDENTITY_TEST_OPTIONAL_TIME"); err == nil {
		t.Fatal("expected invalid optional time to fail")
	}
}
