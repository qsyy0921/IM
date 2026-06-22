package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	tokeninfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/token"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type gatewayTokenSigner interface {
	SignGatewayToken(types.TokenClaims) (string, error)
	JWKSet() tokeninfra.JWKSet
	Issuer() string
}

type gatewayTokenKeyRingSigner struct {
	current gatewayTokenSigner
	jwkSet  tokeninfra.JWKSet
}

func (signer gatewayTokenKeyRingSigner) SignGatewayToken(claims types.TokenClaims) (string, error) {
	return signer.current.SignGatewayToken(claims)
}

func (signer gatewayTokenKeyRingSigner) JWKSet() tokeninfra.JWKSet {
	return signer.jwkSet
}

func (signer gatewayTokenKeyRingSigner) Issuer() string {
	return signer.current.Issuer()
}

type gatewayTokenRS256KeyRingConfig struct {
	Issuer        string                      `json:"issuer,omitempty"`
	Current       gatewayTokenRS256CurrentKey `json:"current"`
	OldPublicKeys []tokeninfra.JWK            `json:"old_public_keys,omitempty"`
}

type gatewayTokenRS256CurrentKey struct {
	KeyID          string `json:"kid"`
	PrivateKeyPEM  string `json:"private_key_pem,omitempty"`
	PrivateKeyFile string `json:"private_key_file,omitempty"`
}

func newGatewayTokenSigner() (gatewayTokenSigner, error) {
	secret := envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_SECRET", "")
	switch strings.ToLower(envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT", "legacy")) {
	case "legacy", "hmac", "custom":
		return tokeninfra.NewHMACSigner(secret)
	case "jwt", "jwt-hs256", "hs256":
		return tokeninfra.NewJWTSigner(
			secret,
			envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_KEY_ID", ""),
			envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ISSUER", ""),
		)
	case "jwt-rs256", "rs256":
		if signer, ok, err := loadRS256KeyRingSigner(); err != nil {
			return nil, err
		} else if ok {
			return signer, nil
		}
		privateKeyPEM, err := loadRSAPrivateKeyPEM()
		if err != nil {
			return nil, err
		}
		return tokeninfra.NewRS256SignerFromPEM(
			privateKeyPEM,
			envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_KEY_ID", ""),
			envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ISSUER", ""),
		)
	default:
		return nil, errors.New("unsupported NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT")
	}
}

func loadRS256KeyRingSigner() (gatewayTokenSigner, bool, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING_JSON"))
	if raw == "" {
		path := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING_FILE"))
		if path == "" {
			return nil, false, nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, true, err
		}
		raw = strings.TrimSpace(string(content))
	}
	if raw == "" {
		return nil, false, nil
	}
	var config gatewayTokenRS256KeyRingConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, true, err
	}
	keyID := strings.TrimSpace(config.Current.KeyID)
	if keyID == "" {
		return nil, true, errors.New("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING current.kid is required")
	}
	privateKeyPEM, err := loadRS256KeyRingPrivateKeyPEM(config.Current)
	if err != nil {
		return nil, true, err
	}
	issuer := strings.TrimSpace(config.Issuer)
	if issuer == "" {
		issuer = envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ISSUER", "")
	}
	current, err := tokeninfra.NewRS256SignerFromPEM(privateKeyPEM, keyID, issuer)
	if err != nil {
		return nil, true, err
	}
	publicKeys, err := mergeGatewayTokenPublicJWKSets(
		current.JWKSet(),
		tokeninfra.JWKSet{Keys: config.OldPublicKeys},
	)
	if err != nil {
		return nil, true, err
	}
	return gatewayTokenKeyRingSigner{current: current, jwkSet: publicKeys}, true, nil
}

func loadRS256KeyRingPrivateKeyPEM(current gatewayTokenRS256CurrentKey) (string, error) {
	if pemValue := strings.TrimSpace(current.PrivateKeyPEM); pemValue != "" {
		return pemValue, nil
	}
	path := strings.TrimSpace(current.PrivateKeyFile)
	if path == "" {
		return "", errors.New("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING current.private_key_pem or current.private_key_file is required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func runGatewayTokenKeyRingRotate() error {
	path := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING_FILE"))
	if path == "" {
		return errors.New("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RS256_KEYRING_FILE is required")
	}
	options := gatewayTokenKeyRingRotateOptions{
		NewKeyID:    envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ROTATE_NEW_KID", ""),
		RSABits:     envInt("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ROTATE_RSA_BITS", 2048),
		OldKeyLimit: envInt("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ROTATE_OLD_KEY_LIMIT", 3),
		Now:         time.Now().UTC(),
	}
	rotated, err := rotateRS256KeyRingFile(path, options)
	if err != nil {
		return err
	}
	log.Printf("rotated identity gateway RS256 keyring file=%s current_kid=%s old_public_keys=%d", path, rotated.Current.KeyID, len(rotated.OldPublicKeys))
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_KEYRING_ROTATE_OUTPUT")); outputPath != "" {
		if err := writeGatewayTokenKeyRingRotateOutput(outputPath, rotated, options); err != nil {
			return err
		}
	}
	return nil
}

type gatewayTokenKeyRingRotateOptions struct {
	NewKeyID    string
	RSABits     int
	OldKeyLimit int
	Now         time.Time
}

func rotateRS256KeyRingFile(path string, options gatewayTokenKeyRingRotateOptions) (gatewayTokenRS256KeyRingConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	var config gatewayTokenRS256KeyRingConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	rotated, err := rotateRS256KeyRing(config, options)
	if err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	raw, err := json.MarshalIndent(rotated, "", "  ")
	if err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	raw = append(raw, '\n')
	perm := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	if err := writeFileReplace(path, raw, perm); err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	return rotated, nil
}

func rotateRS256KeyRing(config gatewayTokenRS256KeyRingConfig, options gatewayTokenKeyRingRotateOptions) (gatewayTokenRS256KeyRingConfig, error) {
	oldCurrentKey, err := currentRS256PublicJWK(config.Current)
	if err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	newKeyID := strings.TrimSpace(options.NewKeyID)
	if newKeyID == "" {
		newKeyID = "nexusim-gateway-rs256-" + now.UTC().Format("20060102T150405Z")
	}
	if newKeyID == oldCurrentKey.KeyID {
		return gatewayTokenRS256KeyRingConfig{}, errors.New("new gateway token kid must differ from current kid")
	}
	for _, key := range config.OldPublicKeys {
		publicKey, ok := publicGatewayTokenJWK(key)
		if !ok {
			return gatewayTokenRS256KeyRingConfig{}, errors.New("gateway token keyring old_public_keys may only contain RS256 public keys")
		}
		if publicKey.KeyID == newKeyID {
			return gatewayTokenRS256KeyRingConfig{}, errors.New("new gateway token kid must not already exist in old_public_keys")
		}
	}
	bits := options.RSABits
	if bits == 0 {
		bits = 2048
	}
	if bits < 2048 {
		return gatewayTokenRS256KeyRingConfig{}, errors.New("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ROTATE_RSA_BITS must be at least 2048")
	}
	oldLimit := options.OldKeyLimit
	if oldLimit == 0 {
		oldLimit = 3
	}
	if oldLimit < 0 {
		return gatewayTokenRS256KeyRingConfig{}, errors.New("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ROTATE_OLD_KEY_LIMIT must be non-negative")
	}
	newPrivateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	oldKeys, err := mergeGatewayTokenPublicJWKSets(
		tokeninfra.JWKSet{Keys: []tokeninfra.JWK{oldCurrentKey}},
		tokeninfra.JWKSet{Keys: config.OldPublicKeys},
	)
	if err != nil {
		return gatewayTokenRS256KeyRingConfig{}, err
	}
	if oldLimit < len(oldKeys.Keys) {
		oldKeys.Keys = oldKeys.Keys[:oldLimit]
	}
	return gatewayTokenRS256KeyRingConfig{
		Issuer: strings.TrimSpace(config.Issuer),
		Current: gatewayTokenRS256CurrentKey{
			KeyID:         newKeyID,
			PrivateKeyPEM: marshalRSAPrivateKeyPEM(newPrivateKey),
		},
		OldPublicKeys: oldKeys.Keys,
	}, nil
}

func currentRS256PublicJWK(current gatewayTokenRS256CurrentKey) (tokeninfra.JWK, error) {
	keyID := strings.TrimSpace(current.KeyID)
	if keyID == "" {
		return tokeninfra.JWK{}, errors.New("gateway token keyring current.kid is required")
	}
	privateKeyPEM, err := loadRS256KeyRingPrivateKeyPEM(current)
	if err != nil {
		return tokeninfra.JWK{}, err
	}
	signer, err := tokeninfra.NewRS256SignerFromPEM(privateKeyPEM, keyID, "")
	if err != nil {
		return tokeninfra.JWK{}, err
	}
	jwks := signer.JWKSet()
	if len(jwks.Keys) != 1 {
		return tokeninfra.JWK{}, errors.New("gateway token keyring current key did not produce one public jwk")
	}
	return jwks.Keys[0], nil
}

func marshalRSAPrivateKeyPEM(privateKey *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
}

func writeFileReplace(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err == nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmpPath, path)
}

func loadRSAPrivateKeyPEM() (string, error) {
	if pemValue := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_PEM")); pemValue != "" {
		return pemValue, nil
	}
	path := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_FILE"))
	if path == "" {
		return "", errors.New("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_PEM or NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_FILE is required for RS256")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func gatewayTokenJWKSetWithAdditionalKeys(base tokeninfra.JWKSet) (tokeninfra.JWKSet, error) {
	additional, err := loadAdditionalGatewayTokenJWKSet()
	if err != nil {
		return tokeninfra.JWKSet{}, err
	}
	return mergeGatewayTokenPublicJWKSets(base, additional)
}

func mergeGatewayTokenPublicJWKSets(sets ...tokeninfra.JWKSet) (tokeninfra.JWKSet, error) {
	totalKeys := 0
	for _, set := range sets {
		totalKeys += len(set.Keys)
	}
	result := tokeninfra.JWKSet{Keys: make([]tokeninfra.JWK, 0, totalKeys)}
	seen := make(map[string]struct{}, totalKeys)
	appendKey := func(key tokeninfra.JWK) error {
		publicKey, ok := publicGatewayTokenJWK(key)
		if !ok {
			return errors.New("gateway token JWKS may only expose RS256 public keys")
		}
		if _, ok := seen[publicKey.KeyID]; ok {
			return nil
		}
		seen[publicKey.KeyID] = struct{}{}
		result.Keys = append(result.Keys, publicKey)
		return nil
	}
	for _, set := range sets {
		for _, key := range set.Keys {
			if err := appendKey(key); err != nil {
				return tokeninfra.JWKSet{}, err
			}
		}
	}
	return result, nil
}

func publicGatewayTokenJWK(key tokeninfra.JWK) (tokeninfra.JWK, bool) {
	publicKey := tokeninfra.JWK{
		KeyType:   strings.TrimSpace(key.KeyType),
		KeyUse:    strings.TrimSpace(key.KeyUse),
		KeyID:     strings.TrimSpace(key.KeyID),
		Algorithm: strings.TrimSpace(key.Algorithm),
		Modulus:   strings.TrimSpace(key.Modulus),
		Exponent:  strings.TrimSpace(key.Exponent),
	}
	if publicKey.KeyType != "RSA" || publicKey.Algorithm != "RS256" || publicKey.KeyID == "" {
		return tokeninfra.JWK{}, false
	}
	if publicKey.KeyUse != "" && publicKey.KeyUse != "sig" {
		return tokeninfra.JWK{}, false
	}
	if publicKey.Modulus == "" || publicKey.Exponent == "" || hasGatewayTokenPrivateJWKMaterial(key) {
		return tokeninfra.JWK{}, false
	}
	modulus, err := base64.RawURLEncoding.DecodeString(publicKey.Modulus)
	if err != nil || len(modulus) == 0 || new(big.Int).SetBytes(modulus).BitLen() < 2048 {
		return tokeninfra.JWK{}, false
	}
	exponent, err := base64.RawURLEncoding.DecodeString(publicKey.Exponent)
	exponentInt := new(big.Int).SetBytes(exponent)
	if err != nil || len(exponent) == 0 || !exponentInt.IsInt64() || exponentInt.Int64() <= 1 {
		return tokeninfra.JWK{}, false
	}
	return publicKey, true
}

func hasGatewayTokenPrivateJWKMaterial(key tokeninfra.JWK) bool {
	return strings.TrimSpace(key.Key) != "" ||
		strings.TrimSpace(key.PrivateExponent) != "" ||
		strings.TrimSpace(key.Prime1) != "" ||
		strings.TrimSpace(key.Prime2) != "" ||
		strings.TrimSpace(key.Exponent1) != "" ||
		strings.TrimSpace(key.Exponent2) != "" ||
		strings.TrimSpace(key.Coefficient) != "" ||
		strings.TrimSpace(string(key.OtherPrimes)) != ""
}

func loadAdditionalGatewayTokenJWKSet() (tokeninfra.JWKSet, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_JSON"))
	if raw == "" {
		path := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_FILE"))
		if path == "" {
			return tokeninfra.JWKSet{}, nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return tokeninfra.JWKSet{}, err
		}
		raw = strings.TrimSpace(string(content))
	}
	if raw == "" {
		return tokeninfra.JWKSet{}, nil
	}
	var set tokeninfra.JWKSet
	if err := json.Unmarshal([]byte(raw), &set); err != nil {
		return tokeninfra.JWKSet{}, err
	}
	return set, nil
}
