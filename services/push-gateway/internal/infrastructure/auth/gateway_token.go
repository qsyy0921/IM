package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

type Mode = gatewayauth.Mode

const (
	ModeMock = gatewayauth.ModeMock
	ModeHMAC = gatewayauth.ModeHMAC
	ModeJWT  = gatewayauth.ModeJWT
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
	inner *gatewayauth.Authenticator
}

type JWKStats = gatewayauth.JWKStats

func NewAuthenticator(config Config) (*Authenticator, error) {
	inner, err := gatewayauth.NewAuthenticator(gatewayauth.Config{
		Mode:               config.Mode,
		Secret:             config.Secret,
		PreviousSecrets:    config.PreviousSecrets,
		JWKSetJSON:         config.JWKSetJSON,
		JWKSetURL:          config.JWKSetURL,
		JWKHTTPClient:      config.JWKHTTPClient,
		JWKRefreshInterval: config.JWKRefreshInterval,
		TrustedIssuers:     config.TrustedIssuers,
		Audience:           config.Audience,
		Revocation:         revocationAdapter{inner: config.Revocation},
		Now:                config.Now,
	})
	if err != nil {
		return nil, err
	}
	return &Authenticator{inner: inner}, nil
}

func (authenticator *Authenticator) Close() {
	if authenticator == nil || authenticator.inner == nil {
		return
	}
	authenticator.inner.Close()
}

func (authenticator *Authenticator) JWKStats() JWKStats {
	if authenticator == nil || authenticator.inner == nil {
		return JWKStats{}
	}
	return authenticator.inner.JWKStats()
}

func (authenticator *Authenticator) Authenticate(request *http.Request) (types.AuthContext, error) {
	if authenticator == nil || authenticator.inner == nil {
		auth, err := gatewayauth.NewAuthenticator(gatewayauth.Config{Mode: gatewayauth.ModeMock})
		if err != nil {
			return types.AuthContext{}, mapAuthError(err)
		}
		shared, err := auth.Authenticate(request)
		return toPushAuth(shared), mapAuthError(err)
	}
	shared, err := authenticator.inner.Authenticate(request)
	return toPushAuth(shared), mapAuthError(err)
}

func SignGatewayToken(secret string, claims map[string]string, expiresAt time.Time) (string, error) {
	return gatewayauth.SignGatewayToken(secret, claims, expiresAt)
}

func SignGatewayJWT(secret string, claims map[string]string, expiresAt time.Time) (string, error) {
	return gatewayauth.SignGatewayJWT(secret, claims, expiresAt)
}

func ClaimsFromTenantUserDevice(tenantID string, userID string, deviceID string) map[string]string {
	return gatewayauth.ClaimsFromTenantUserDevice(tenantID, userID, deviceID)
}

type revocationAdapter struct {
	inner RevocationChecker
}

func (adapter revocationAdapter) IsRevoked(ctx context.Context, auth gatewayauth.AuthContext) (bool, error) {
	if adapter.inner == nil {
		return false, nil
	}
	return adapter.inner.IsRevoked(ctx, toPushAuth(auth))
}

func toPushAuth(auth gatewayauth.AuthContext) types.AuthContext {
	return types.AuthContext{
		TenantID:  auth.TenantID,
		UserID:    auth.UserID,
		DeviceID:  auth.DeviceID,
		SessionID: auth.SessionID,
		TraceID:   auth.TraceID,
		RequestID: auth.RequestID,
	}
}

func mapAuthError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gatewayauth.ErrAuthExpired):
		return types.NewAuthExpired("token expired")
	case errors.Is(err, gatewayauth.ErrInvalidRequest):
		return types.NewInvalidFrame(err.Error())
	case errors.Is(err, gatewayauth.ErrPermissionDenied):
		return types.ErrPermissionDenied
	default:
		return err
	}
}
