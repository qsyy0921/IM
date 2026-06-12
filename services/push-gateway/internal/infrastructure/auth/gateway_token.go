package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

type Mode string

const (
	ModeMock Mode = "mock"
	ModeHMAC Mode = "hmac"
)

type Config struct {
	Mode            Mode
	Secret          string
	PreviousSecrets []string
	Audience        string
	Revocation      RevocationChecker
	Now             func() time.Time
}

type RevocationChecker interface {
	IsRevoked(context.Context, types.AuthContext) (bool, error)
}

type Authenticator struct {
	mode       Mode
	secrets    [][]byte
	audience   string
	revocation RevocationChecker
	now        func() time.Time
}

type tokenClaims struct {
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

func NewAuthenticator(config Config) (*Authenticator, error) {
	mode := config.Mode
	if mode == "" {
		mode = ModeMock
	}
	audience := strings.TrimSpace(config.Audience)
	if audience == "" {
		audience = "push-gateway"
	}
	authenticator := &Authenticator{mode: mode, audience: audience, revocation: config.Revocation, now: config.Now}
	if authenticator.now == nil {
		authenticator.now = time.Now
	}
	switch mode {
	case ModeMock:
		return authenticator, nil
	case ModeHMAC:
		secret := strings.TrimSpace(config.Secret)
		if secret == "" {
			return nil, errors.New("NEXUSIM_PUSH_AUTH_HMAC_SECRET is required when NEXUSIM_PUSH_AUTH_MODE=hmac")
		}
		seenSecrets := map[string]struct{}{secret: {}}
		authenticator.secrets = append(authenticator.secrets, []byte(secret))
		for _, previous := range config.PreviousSecrets {
			previous = strings.TrimSpace(previous)
			if previous == "" {
				continue
			}
			if _, ok := seenSecrets[previous]; ok {
				continue
			}
			seenSecrets[previous] = struct{}{}
			authenticator.secrets = append(authenticator.secrets, []byte(previous))
		}
		return authenticator, nil
	default:
		return nil, errors.New("unsupported NEXUSIM_PUSH_AUTH_MODE")
	}
}

func (authenticator *Authenticator) Authenticate(request *http.Request) (types.AuthContext, error) {
	if authenticator == nil {
		return authenticateMock(request)
	}
	switch authenticator.mode {
	case ModeMock:
		return authenticateMock(request)
	case ModeHMAC:
		return authenticator.authenticateHMAC(request)
	default:
		return types.AuthContext{}, types.ErrPermissionDenied
	}
}

func (authenticator *Authenticator) authenticateHMAC(request *http.Request) (types.AuthContext, error) {
	query := request.URL.Query()
	token := strings.TrimSpace(query.Get("token"))
	if token == "" {
		token = bearerToken(request.Header.Get("Authorization"))
	}
	if token == "" {
		return types.AuthContext{}, types.ErrPermissionDenied
	}
	claims, err := authenticator.parseToken(token)
	if err != nil {
		return types.AuthContext{}, err
	}
	auth := types.AuthContext{
		TenantID:  claims.TenantID,
		UserID:    claims.UserID,
		DeviceID:  claims.DeviceID,
		SessionID: claims.SessionID,
		TraceID:   firstNonEmpty(claims.TraceID, query.Get("trace_id")),
	}
	if deviceID := strings.TrimSpace(query.Get("device_id")); deviceID != "" {
		if auth.DeviceID != "" && auth.DeviceID != deviceID {
			return types.AuthContext{}, types.ErrPermissionDenied
		}
		auth.DeviceID = deviceID
	}
	if auth.TenantID == "" || auth.UserID == "" {
		return types.AuthContext{}, types.ErrPermissionDenied
	}
	if authenticator.revocation != nil {
		revoked, err := authenticator.revocation.IsRevoked(request.Context(), auth)
		if err != nil || revoked {
			return types.AuthContext{}, types.ErrPermissionDenied
		}
	}
	return auth, nil
}

func (authenticator *Authenticator) parseToken(token string) (tokenClaims, error) {
	parts := strings.Split(token, ".")
	switch len(parts) {
	case 2:
		return authenticator.parseLegacyToken(parts)
	case 3:
		return authenticator.parseJWT(parts)
	default:
		return tokenClaims{}, types.ErrPermissionDenied
	}
}

func (authenticator *Authenticator) parseLegacyToken(parts []string) (tokenClaims, error) {
	if parts[0] == "" || parts[1] == "" {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	if !authenticator.validSignature(parts[0], signature) {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	return authenticator.parseClaims(payloadBytes)
}

func (authenticator *Authenticator) parseJWT(parts []string) (tokenClaims, error) {
	if parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	var header tokenHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	if header.Algorithm != "HS256" || header.Type != "JWT" {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	if !authenticator.validSignature(parts[0]+"."+parts[1], signature) {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	return authenticator.parseClaims(payloadBytes)
}

func (authenticator *Authenticator) parseClaims(payloadBytes []byte) (tokenClaims, error) {
	var claims tokenClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	if claims.TenantID == "" || claims.UserID == "" || claims.Audience != authenticator.audience {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	if claims.Expires <= 0 || authenticator.now().Unix() >= claims.Expires {
		return tokenClaims{}, types.NewAuthExpired("token expired")
	}
	return claims, nil
}

func (authenticator *Authenticator) validSignature(payload string, signature []byte) bool {
	for _, secret := range authenticator.secrets {
		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write([]byte(payload))
		if hmac.Equal(signature, mac.Sum(nil)) {
			return true
		}
	}
	return false
}

func authenticateMock(request *http.Request) (types.AuthContext, error) {
	query := request.URL.Query()
	auth := types.AuthContext{
		TenantID: query.Get("tenant_id"),
		UserID:   query.Get("user_id"),
		DeviceID: query.Get("device_id"),
		TraceID:  query.Get("trace_id"),
	}
	if token := strings.TrimSpace(query.Get("token")); token != "" && (auth.TenantID == "" || auth.UserID == "") {
		parts := strings.Split(token, ":")
		if len(parts) >= 2 {
			auth.TenantID = parts[0]
			auth.UserID = parts[1]
			if auth.DeviceID == "" && len(parts) >= 3 {
				auth.DeviceID = parts[2]
			}
		}
	}
	if auth.TenantID == "" {
		return types.AuthContext{}, types.NewInvalidFrame("tenant_id is required")
	}
	if auth.UserID == "" {
		return types.AuthContext{}, types.NewInvalidFrame("user_id is required")
	}
	return auth, nil
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	prefix := "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func SignGatewayToken(secret string, claims map[string]string, expiresAt time.Time) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", types.ErrPermissionDenied
	}
	payload := tokenClaims{
		TenantID:  strings.TrimSpace(claims["tenant_id"]),
		UserID:    strings.TrimSpace(claims["user_id"]),
		DeviceID:  strings.TrimSpace(claims["device_id"]),
		SessionID: strings.TrimSpace(claims["session_id"]),
		TraceID:   strings.TrimSpace(claims["trace_id"]),
		Audience:  firstNonEmpty(claims["aud"], "push-gateway"),
		Expires:   expiresAt.Unix(),
	}
	if payload.TenantID == "" || payload.UserID == "" || payload.Expires <= 0 {
		return "", types.ErrPermissionDenied
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payloadBytes)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payloadPart))
	return payloadPart + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func SignGatewayJWT(secret string, claims map[string]string, expiresAt time.Time) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", types.ErrPermissionDenied
	}
	payload := tokenClaims{
		TenantID:  strings.TrimSpace(claims["tenant_id"]),
		UserID:    strings.TrimSpace(claims["user_id"]),
		DeviceID:  strings.TrimSpace(claims["device_id"]),
		SessionID: strings.TrimSpace(claims["session_id"]),
		TraceID:   strings.TrimSpace(claims["trace_id"]),
		Issuer:    firstNonEmpty(claims["iss"], "nexusim-identity"),
		Subject:   firstNonEmpty(claims["sub"], claims["user_id"]),
		Audience:  firstNonEmpty(claims["aud"], "push-gateway"),
		IssuedAt:  time.Now().Unix(),
		Expires:   expiresAt.Unix(),
	}
	if payload.TenantID == "" || payload.UserID == "" || payload.Expires <= 0 {
		return "", types.ErrPermissionDenied
	}
	header := tokenHeader{
		Algorithm: "HS256",
		Type:      "JWT",
		KeyID:     firstNonEmpty(claims["kid"], "nexusim-local-gateway-hs256"),
	}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerBytes) + "." + base64.RawURLEncoding.EncodeToString(payloadBytes)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func ClaimsFromTenantUserDevice(tenantID string, userID string, deviceID string) map[string]string {
	return map[string]string{
		"tenant_id": tenantID,
		"user_id":   userID,
		"device_id": deviceID,
		"trace_id":  "trace-" + strconv.FormatInt(time.Now().UnixNano(), 10),
	}
}
