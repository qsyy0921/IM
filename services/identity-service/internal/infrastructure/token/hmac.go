package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/qsyy0921/IM/services/identity-service/internal/domain"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type HMACSigner struct {
	secret []byte
	format tokenFormat
	keyID  string
	issuer string
}

type gatewayClaims struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	DeviceID  string `json:"device_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
	Issuer    string `json:"iss,omitempty"`
	Subject   string `json:"sub,omitempty"`
	Audience  string `json:"aud"`
	IssuedAt  int64  `json:"iat,omitempty"`
	Expires   int64  `json:"exp"`
}

type tokenHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
	KeyID     string `json:"kid,omitempty"`
}

type tokenFormat string

const (
	tokenFormatLegacy tokenFormat = "legacy"
	tokenFormatJWT    tokenFormat = "jwt"
)

const (
	defaultKeyID  = "nexusim-local-gateway-hs256"
	defaultIssuer = "nexusim-identity"
)

type JWKSet struct {
	Keys []JWK `json:"keys"`
}

type JWK struct {
	KeyType         string          `json:"kty"`
	KeyUse          string          `json:"use,omitempty"`
	KeyID           string          `json:"kid,omitempty"`
	Algorithm       string          `json:"alg,omitempty"`
	Key             string          `json:"k,omitempty"`
	Modulus         string          `json:"n,omitempty"`
	Exponent        string          `json:"e,omitempty"`
	PrivateExponent string          `json:"d,omitempty"`
	Prime1          string          `json:"p,omitempty"`
	Prime2          string          `json:"q,omitempty"`
	Exponent1       string          `json:"dp,omitempty"`
	Exponent2       string          `json:"dq,omitempty"`
	Coefficient     string          `json:"qi,omitempty"`
	OtherPrimes     json.RawMessage `json:"oth,omitempty"`
}

func NewHMACSigner(secret string) (*HMACSigner, error) {
	return newHMACSigner(secret, tokenFormatLegacy, "", "")
}

func NewJWTSigner(secret string, keyID string, issuer string) (*HMACSigner, error) {
	return newHMACSigner(secret, tokenFormatJWT, keyID, issuer)
}

func newHMACSigner(secret string, format tokenFormat, keyID string, issuer string) (*HMACSigner, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, types.NewTokenSigningFailed("hmac secret is required")
	}
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		keyID = defaultKeyID
	}
	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		issuer = defaultIssuer
	}
	return &HMACSigner{secret: []byte(secret), format: format, keyID: keyID, issuer: issuer}, nil
}

func (signer *HMACSigner) SignGatewayToken(claims types.TokenClaims) (string, error) {
	if signer == nil || len(signer.secret) == 0 {
		return "", types.NewTokenSigningFailed("hmac signer is not configured")
	}
	audience := strings.TrimSpace(claims.Audience)
	if audience == "" {
		audience = domain.DefaultGatewayAudience
	}
	payload := gatewayClaims{
		TenantID:  string(claims.TenantID),
		UserID:    string(claims.UserID),
		DeviceID:  string(claims.DeviceID),
		SessionID: string(claims.SessionID),
		TraceID:   claims.TraceID,
		Issuer:    claims.Issuer,
		Audience:  audience,
		IssuedAt:  claims.IssuedAt,
		Expires:   claims.ExpiresAt,
	}
	if payload.TenantID == "" || payload.UserID == "" || payload.DeviceID == "" || payload.Audience == "" || payload.Expires <= 0 {
		return "", types.NewTokenSigningFailed("gateway token claims are incomplete")
	}
	if signer.format == tokenFormatJWT {
		return signer.signJWT(payload)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewTokenSigningFailed(err.Error())
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, signer.secret)
	_, _ = mac.Write([]byte(payloadPart))
	return payloadPart + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (signer *HMACSigner) signJWT(payload gatewayClaims) (string, error) {
	if payload.Issuer == "" {
		payload.Issuer = signer.issuer
	}
	if payload.Subject == "" {
		payload.Subject = payload.UserID
	}
	header := tokenHeader{Algorithm: "HS256", Type: "JWT", KeyID: signer.keyID}
	headerRaw, err := json.Marshal(header)
	if err != nil {
		return "", types.NewTokenSigningFailed(err.Error())
	}
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewTokenSigningFailed(err.Error())
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerRaw) + "." + base64.RawURLEncoding.EncodeToString(payloadRaw)
	mac := hmac.New(sha256.New, signer.secret)
	_, _ = mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (signer *HMACSigner) JWKSet() JWKSet {
	return JWKSet{Keys: []JWK{}}
}

func (signer *HMACSigner) Issuer() string {
	if signer == nil {
		return ""
	}
	return signer.issuer
}
