package main

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/app"
	moderationinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/moderation"
)

func policyContentModeratorFromEnv() (app.MessageContentModerator, bool, error) {
	mode := strings.TrimSpace(strings.ToLower(os.Getenv("NEXUSIM_POLICY_MODERATION_MODE")))
	switch mode {
	case "", "disabled", "noop":
		return nil, false, nil
	case "keyword":
		terms := splitCSV(os.Getenv("NEXUSIM_POLICY_MODERATION_DENY_TERMS"))
		if len(terms) == 0 {
			return nil, true, errors.New("NEXUSIM_POLICY_MODERATION_DENY_TERMS is required when keyword moderation is enabled")
		}
		return moderationinfra.NewKeywordModerator(moderationinfra.KeywordConfig{
			DenyTerms:         terms,
			PermissionVersion: envInt64("NEXUSIM_POLICY_MODERATION_PERMISSION_VERSION", 1),
			Classification:    envString("NEXUSIM_POLICY_MODERATION_CLASSIFICATION", "CONTENT_MODERATION_DENIED"),
			Reason:            envString("NEXUSIM_POLICY_MODERATION_DENY_REASON", "content moderation policy denied"),
		}), true, nil
	case "http":
		endpoint := envString("NEXUSIM_POLICY_MODERATION_HTTP_ENDPOINT", "")
		if err := validatePolicyModerationHTTPEndpoint(endpoint, envBool("NEXUSIM_POLICY_MODERATION_HTTP_ALLOW_INSECURE", false)); err != nil {
			return nil, true, err
		}
		moderator, err := moderationinfra.NewHTTPModerator(moderationinfra.HTTPConfig{
			Endpoint:          endpoint,
			BearerToken:       envString("NEXUSIM_POLICY_MODERATION_HTTP_BEARER_TOKEN", ""),
			Timeout:           envDuration("NEXUSIM_POLICY_MODERATION_HTTP_TIMEOUT", time.Second),
			PermissionVersion: envInt64("NEXUSIM_POLICY_MODERATION_PERMISSION_VERSION", 1),
			Classification:    envString("NEXUSIM_POLICY_MODERATION_CLASSIFICATION", "CONTENT_PROVIDER_DENIED"),
			Reason:            envString("NEXUSIM_POLICY_MODERATION_DENY_REASON", "content moderation provider denied"),
		})
		if err != nil {
			return nil, true, err
		}
		return moderator, true, nil
	default:
		return nil, true, errors.New("unsupported NEXUSIM_POLICY_MODERATION_MODE")
	}
}

func validatePolicyModerationHTTPEndpoint(endpoint string, allowInsecure bool) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return errors.New("NEXUSIM_POLICY_MODERATION_HTTP_ENDPOINT is required when http moderation is enabled")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("NEXUSIM_POLICY_MODERATION_HTTP_ENDPOINT must be an absolute URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return nil
	case "http":
		if allowInsecure {
			return nil
		}
		return errors.New("NEXUSIM_POLICY_MODERATION_HTTP_ALLOW_INSECURE=true is required for http moderation endpoint")
	default:
		return errors.New("NEXUSIM_POLICY_MODERATION_HTTP_ENDPOINT must use https")
	}
}
