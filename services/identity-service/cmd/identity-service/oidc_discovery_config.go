package main

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	monitoringinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/monitoring"
	tokeninfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/token"
)

func identityOIDCDiscoveryFromEnv(signer gatewayTokenSigner, jwkSet tokeninfra.JWKSet) (*monitoringinfra.OIDCDiscovery, error) {
	enabled, configured, err := envOptionalBool("NEXUSIM_IDENTITY_OIDC_DISCOVERY_ENABLED")
	if err != nil {
		return nil, err
	}
	if !configured || !enabled {
		return nil, nil
	}
	if signer == nil {
		return nil, errors.New("OIDC discovery requires gateway token signer")
	}
	if len(jwkSet.Keys) == 0 {
		return nil, errors.New("OIDC discovery requires public RS256 JWKS")
	}
	issuer := strings.TrimRight(strings.TrimSpace(envString("NEXUSIM_IDENTITY_OIDC_ISSUER", signer.Issuer())), "/")
	if err := validateIdentityOIDCURL("NEXUSIM_IDENTITY_OIDC_ISSUER", issuer, true); err != nil {
		return nil, err
	}
	jwksURI := strings.TrimSpace(envString("NEXUSIM_IDENTITY_OIDC_JWKS_URI", issuer+"/.well-known/jwks.json"))
	if err := validateIdentityOIDCURL("NEXUSIM_IDENTITY_OIDC_JWKS_URI", jwksURI, false); err != nil {
		return nil, err
	}
	return &monitoringinfra.OIDCDiscovery{
		Issuer:                           issuer,
		JWKSURI:                          jwksURI,
		ResponseTypesSupported:           []string{"id_token"},
		SubjectTypesSupported:            []string{"public"},
		IDTokenSigningAlgValuesSupported: []string{"RS256"},
		ClaimsSupported: []string{
			"iss",
			"sub",
			"aud",
			"exp",
			"iat",
			"tenant_id",
			"user_id",
			"device_id",
			"session_id",
			"trace_id",
		},
	}, nil
}

func validateIdentityOIDCURL(name string, raw string, issuer bool) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s must be an absolute http or https URL", name)
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		if !listenerAddrTrustedWithoutMTLS(parsed.Hostname()) {
			return fmt.Errorf("%s must use https unless it targets localhost or a private address", name)
		}
	default:
		return fmt.Errorf("%s must use http or https", name)
	}
	if issuer && (parsed.RawQuery != "" || parsed.Fragment != "") {
		return fmt.Errorf("%s must not include query or fragment", name)
	}
	return nil
}
