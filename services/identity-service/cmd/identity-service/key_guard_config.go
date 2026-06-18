package main

import (
	"errors"
	"os"
	"strings"
)

type identityRuntimeKeyGuardScope struct {
	GatewayToken           bool
	MFA                    bool
	MFARecovery            bool
	ChallengeRequestLimit  bool
	ChallengeDeliveryToken bool
}

func validateIdentityProductionKeyGuardFromEnv(scope identityRuntimeKeyGuardScope) error {
	enabled, _, err := envOptionalBool("NEXUSIM_IDENTITY_PRODUCTION_KEY_GUARD")
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	var problems []string
	if scope.GatewayToken {
		format := strings.ToLower(envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT", "legacy"))
		if format != "rs256" && format != "jwt-rs256" {
			problems = append(problems, "NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT must be rs256 or jwt-rs256")
		}
	}
	if scope.MFA && !envAnySet(
		"NEXUSIM_IDENTITY_MFA_SECRET_KEYRING_JSON",
		"NEXUSIM_IDENTITY_MFA_SECRET_KEYRING_FILE",
		"NEXUSIM_IDENTITY_MFA_SECRET_KEY",
	) {
		problems = append(problems, "MFA requires NEXUSIM_IDENTITY_MFA_SECRET_KEYRING_JSON/FILE or NEXUSIM_IDENTITY_MFA_SECRET_KEY")
	}
	if scope.MFARecovery && !envAnySet("NEXUSIM_IDENTITY_MFA_RECOVERY_CODE_SECRET") {
		problems = append(problems, "recovery codes require NEXUSIM_IDENTITY_MFA_RECOVERY_CODE_SECRET")
	}
	if scope.ChallengeRequestLimit && !envAnySet("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_SECRET") {
		problems = append(problems, "challenge request limiting requires NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_SECRET")
	}
	if scope.ChallengeDeliveryToken && !envAnySet(
		"NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEYRING_JSON",
		"NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEYRING_FILE",
		"NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEY",
	) {
		problems = append(problems, "challenge delivery outbox requires NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEYRING_JSON/FILE or NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEY")
	}
	if len(problems) > 0 {
		return errors.New("identity production key guard failed: " + strings.Join(problems, "; "))
	}
	return nil
}

func envAnySet(names ...string) bool {
	for _, name := range names {
		if strings.TrimSpace(strings.Trim(os.Getenv(name), "\ufeff")) != "" {
			return true
		}
	}
	return false
}
