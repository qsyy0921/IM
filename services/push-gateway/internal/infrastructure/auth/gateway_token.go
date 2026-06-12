package auth

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

type Mode string

const (
	ModeMock Mode = "mock"
	ModeHMAC Mode = "hmac"
	ModeJWT  Mode = "jwt"
)

type Config struct {
	Mode               Mode
	Secret             string
	PreviousSecrets    []string
	JWKSetJSON         string
	JWKSetURL          string
	JWKHTTPClient      *http.Client
	JWKRefreshInterval time.Duration
	TrustedIssuers     []string
	Audience           string
	Revocation         RevocationChecker
	Now                func() time.Time
}

type RevocationChecker interface {
	IsRevoked(context.Context, types.AuthContext) (bool, error)
}

type Authenticator struct {
	mode               Mode
	secrets            [][]byte
	keysMu             sync.RWMutex
	publicKeys         map[string]*rsa.PublicKey
	issuers            map[string]struct{}
	audience           string
	revocation         RevocationChecker
	now                func() time.Time
	jwkSetURL          string
	jwkHTTPClient      *http.Client
	jwkRefreshInterval time.Duration
	jwkRefreshCancel   context.CancelFunc
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

type jwkSet struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	KeyType   string `json:"kty"`
	KeyUse    string `json:"use,omitempty"`
	KeyID     string `json:"kid,omitempty"`
	Algorithm string `json:"alg,omitempty"`
	Modulus   string `json:"n,omitempty"`
	Exponent  string `json:"e,omitempty"`
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
	case ModeJWT:
		authenticator.issuers = normalizeIssuers(config.TrustedIssuers)
		if strings.TrimSpace(config.JWKSetJSON) != "" {
			keys, err := parseRS256JWKSet(config.JWKSetJSON)
			if err != nil {
				return nil, err
			}
			authenticator.setPublicKeys(keys)
		}
		authenticator.jwkSetURL = strings.TrimSpace(config.JWKSetURL)
		if authenticator.jwkSetURL != "" {
			authenticator.jwkHTTPClient = config.JWKHTTPClient
			if authenticator.jwkHTTPClient == nil {
				authenticator.jwkHTTPClient = &http.Client{Timeout: 5 * time.Second}
			}
			authenticator.jwkRefreshInterval = config.JWKRefreshInterval
			if authenticator.jwkRefreshInterval <= 0 {
				authenticator.jwkRefreshInterval = 5 * time.Minute
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			keys, err := authenticator.fetchRS256JWKSet(ctx)
			cancel()
			if err != nil {
				if authenticator.publicKeyCount() == 0 {
					return nil, err
				}
			} else {
				authenticator.setPublicKeys(keys)
			}
			authenticator.startJWKRefresh()
		}
		if authenticator.publicKeyCount() == 0 {
			return nil, errors.New("NEXUSIM_PUSH_AUTH_JWKS_JSON, NEXUSIM_PUSH_AUTH_JWKS_FILE, or NEXUSIM_PUSH_AUTH_JWKS_URL is required when NEXUSIM_PUSH_AUTH_MODE=jwt")
		}
		return authenticator, nil
	default:
		return nil, errors.New("unsupported NEXUSIM_PUSH_AUTH_MODE")
	}
}

func (authenticator *Authenticator) Close() {
	if authenticator == nil || authenticator.jwkRefreshCancel == nil {
		return
	}
	authenticator.jwkRefreshCancel()
}

func (authenticator *Authenticator) Authenticate(request *http.Request) (types.AuthContext, error) {
	if authenticator == nil {
		return authenticateMock(request)
	}
	switch authenticator.mode {
	case ModeMock:
		return authenticateMock(request)
	case ModeHMAC:
		return authenticator.authenticateSignedToken(request)
	case ModeJWT:
		return authenticator.authenticateSignedToken(request)
	default:
		return types.AuthContext{}, types.ErrPermissionDenied
	}
}

func (authenticator *Authenticator) authenticateSignedToken(request *http.Request) (types.AuthContext, error) {
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
		if authenticator.mode != ModeHMAC {
			return tokenClaims{}, types.ErrPermissionDenied
		}
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
	if !authenticator.validHMACSignature(parts[0], signature) {
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
	if header.Type != "JWT" {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return tokenClaims{}, types.ErrPermissionDenied
	}
	switch authenticator.mode {
	case ModeHMAC:
		if header.Algorithm != "HS256" || !authenticator.validHMACSignature(parts[0]+"."+parts[1], signature) {
			return tokenClaims{}, types.ErrPermissionDenied
		}
	case ModeJWT:
		if header.Algorithm != "RS256" || !authenticator.validRS256Signature(header.KeyID, parts[0]+"."+parts[1], signature) {
			return tokenClaims{}, types.ErrPermissionDenied
		}
	default:
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
	if len(authenticator.issuers) > 0 {
		if _, ok := authenticator.issuers[claims.Issuer]; !ok {
			return tokenClaims{}, types.ErrPermissionDenied
		}
	}
	if claims.Expires <= 0 || authenticator.now().Unix() >= claims.Expires {
		return tokenClaims{}, types.NewAuthExpired("token expired")
	}
	return claims, nil
}

func (authenticator *Authenticator) validHMACSignature(payload string, signature []byte) bool {
	for _, secret := range authenticator.secrets {
		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write([]byte(payload))
		if hmac.Equal(signature, mac.Sum(nil)) {
			return true
		}
	}
	return false
}

func (authenticator *Authenticator) validRS256Signature(keyID string, payload string, signature []byte) bool {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return false
	}
	publicKey := authenticator.publicKey(keyID)
	if publicKey == nil {
		return false
	}
	digest := sha256.Sum256([]byte(payload))
	return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature) == nil
}

func (authenticator *Authenticator) publicKey(keyID string) *rsa.PublicKey {
	authenticator.keysMu.RLock()
	defer authenticator.keysMu.RUnlock()
	return authenticator.publicKeys[keyID]
}

func (authenticator *Authenticator) publicKeyCount() int {
	authenticator.keysMu.RLock()
	defer authenticator.keysMu.RUnlock()
	return len(authenticator.publicKeys)
}

func (authenticator *Authenticator) setPublicKeys(keys map[string]*rsa.PublicKey) {
	authenticator.keysMu.Lock()
	defer authenticator.keysMu.Unlock()
	authenticator.publicKeys = keys
}

func (authenticator *Authenticator) fetchRS256JWKSet(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	if authenticator.jwkHTTPClient == nil {
		authenticator.jwkHTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, authenticator.jwkSetURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := authenticator.jwkHTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errors.New("jwks endpoint returned non-success status")
	}
	var set jwkSet
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&set); err != nil {
		return nil, err
	}
	return parseRS256JWKSetValue(set)
}

func (authenticator *Authenticator) startJWKRefresh() {
	ctx, cancel := context.WithCancel(context.Background())
	authenticator.jwkRefreshCancel = cancel
	go func() {
		ticker := time.NewTicker(authenticator.jwkRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				refreshCtx, refreshCancel := context.WithTimeout(ctx, 5*time.Second)
				keys, err := authenticator.fetchRS256JWKSet(refreshCtx)
				refreshCancel()
				if err == nil {
					authenticator.setPublicKeys(keys)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func parseRS256JWKSet(raw string) (map[string]*rsa.PublicKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("NEXUSIM_PUSH_AUTH_JWKS_JSON is required when NEXUSIM_PUSH_AUTH_MODE=jwt")
	}
	var set jwkSet
	if err := json.Unmarshal([]byte(raw), &set); err != nil {
		return nil, err
	}
	return parseRS256JWKSetValue(set)
}

func parseRS256JWKSetValue(set jwkSet) (map[string]*rsa.PublicKey, error) {
	keys := make(map[string]*rsa.PublicKey)
	for _, key := range set.Keys {
		if key.KeyType != "RSA" || key.Algorithm != "RS256" || strings.TrimSpace(key.KeyID) == "" {
			continue
		}
		modulus, err := base64.RawURLEncoding.DecodeString(key.Modulus)
		if err != nil || len(modulus) == 0 {
			continue
		}
		exponentBytes, err := base64.RawURLEncoding.DecodeString(key.Exponent)
		if err != nil || len(exponentBytes) == 0 {
			continue
		}
		exponent := int(new(big.Int).SetBytes(exponentBytes).Int64())
		if exponent <= 1 {
			continue
		}
		keys[key.KeyID] = &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}
	}
	if len(keys) == 0 {
		return nil, errors.New("no usable RS256 JWK keys configured")
	}
	return keys, nil
}

func normalizeIssuers(values []string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result
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
