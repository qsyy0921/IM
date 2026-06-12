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
}

type gatewayClaims struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	DeviceID  string `json:"device_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
	Audience  string `json:"aud"`
	Expires   int64  `json:"exp"`
}

func NewHMACSigner(secret string) (*HMACSigner, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, types.NewTokenSigningFailed("hmac secret is required")
	}
	return &HMACSigner{secret: []byte(secret)}, nil
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
		Audience:  audience,
		Expires:   claims.ExpiresAt,
	}
	if payload.TenantID == "" || payload.UserID == "" || payload.DeviceID == "" || payload.Audience == "" || payload.Expires <= 0 {
		return "", types.NewTokenSigningFailed("gateway token claims are incomplete")
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
