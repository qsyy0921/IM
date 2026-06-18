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
	"math/big"
	"strings"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

const defaultRSAKeyID = "nexusim-gateway-rs256"

type RS256Signer struct {
	privateKey *rsa.PrivateKey
	keyID      string
	issuer     string
}

func NewRS256SignerFromPEM(privateKeyPEM string, keyID string, issuer string) (*RS256Signer, error) {
	privateKey, err := parseRSAPrivateKeyPEM(privateKeyPEM)
	if err != nil {
		return nil, types.NewTokenSigningFailed(err.Error())
	}
	if privateKey.N == nil || privateKey.N.BitLen() < 2048 {
		return nil, types.NewTokenSigningFailed("rsa private key must be at least 2048 bits")
	}
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		keyID = defaultRSAKeyID
	}
	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		issuer = defaultIssuer
	}
	return &RS256Signer{privateKey: privateKey, keyID: keyID, issuer: issuer}, nil
}

func (signer *RS256Signer) SignGatewayToken(claims types.TokenClaims) (string, error) {
	if signer == nil || signer.privateKey == nil {
		return "", types.NewTokenSigningFailed("rs256 signer is not configured")
	}
	payload := gatewayClaims{
		TenantID:  string(claims.TenantID),
		UserID:    string(claims.UserID),
		DeviceID:  string(claims.DeviceID),
		SessionID: string(claims.SessionID),
		TraceID:   claims.TraceID,
		Issuer:    firstNonEmpty(claims.Issuer, signer.issuer),
		Subject:   string(claims.UserID),
		Audience:  firstNonEmpty(claims.Audience, "push-gateway"),
		IssuedAt:  claims.IssuedAt,
		Expires:   claims.ExpiresAt,
	}
	if payload.TenantID == "" || payload.UserID == "" || payload.DeviceID == "" || payload.Audience == "" || payload.Expires <= 0 {
		return "", types.NewTokenSigningFailed("gateway token claims are incomplete")
	}
	header := tokenHeader{Algorithm: "RS256", Type: "JWT", KeyID: signer.keyID}
	headerRaw, err := json.Marshal(header)
	if err != nil {
		return "", types.NewTokenSigningFailed(err.Error())
	}
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewTokenSigningFailed(err.Error())
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerRaw) + "." + base64.RawURLEncoding.EncodeToString(payloadRaw)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, signer.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", types.NewTokenSigningFailed(err.Error())
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (signer *RS256Signer) JWKSet() JWKSet {
	if signer == nil || signer.privateKey == nil {
		return JWKSet{Keys: []JWK{}}
	}
	publicKey := signer.privateKey.PublicKey
	return JWKSet{Keys: []JWK{{
		KeyType:   "RSA",
		KeyUse:    "sig",
		KeyID:     signer.keyID,
		Algorithm: "RS256",
		Modulus:   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
		Exponent:  base64.RawURLEncoding.EncodeToString(big.NewInt(int64(publicKey.E)).Bytes()),
	}}}
}

func (signer *RS256Signer) Issuer() string {
	if signer == nil {
		return ""
	}
	return signer.issuer
}

func parseRSAPrivateKeyPEM(privateKeyPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(privateKeyPEM)))
	if block == nil {
		return nil, types.NewTokenSigningFailed("rsa private key pem is required")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, types.NewTokenSigningFailed("private key is not RSA")
	}
	return key, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
