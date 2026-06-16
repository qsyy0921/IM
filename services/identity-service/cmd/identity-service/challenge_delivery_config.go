package main

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/app"
	notificationinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/notification"
	tokeninfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/token"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func newChallengeNotifier() (app.ChallengeNotifier, string, error) {
	mode := challengeDeliveryMode()
	switch mode {
	case "", "noop", "disabled":
		return notificationinfra.NewNoopChallengeNotifier(), mode, nil
	case "outbox":
		return notificationinfra.NewNoopChallengeNotifier(), mode, nil
	case "webhook":
		notifier, err := newChallengeWebhookNotifier()
		return notifier, mode, err
	case "smtp":
		notifier, err := newChallengeSMTPNotifier()
		return notifier, mode, err
	default:
		return nil, mode, errors.New("unsupported NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_MODE")
	}
}

func newChallengeWebhookNotifier() (app.ChallengeNotifier, error) {
	return notificationinfra.NewWebhookChallengeNotifier(
		envString("NEXUSIM_IDENTITY_CHALLENGE_WEBHOOK_URL", ""),
		envString("NEXUSIM_IDENTITY_CHALLENGE_WEBHOOK_BEARER_TOKEN", ""),
		envDuration("NEXUSIM_IDENTITY_CHALLENGE_WEBHOOK_TIMEOUT", 5*time.Second),
	)
}

func newChallengeSMTPNotifier() (app.ChallengeNotifier, error) {
	return notificationinfra.NewSMTPChallengeNotifier(notificationinfra.SMTPChallengeNotifierConfig{
		Addr:          envString("NEXUSIM_IDENTITY_CHALLENGE_SMTP_ADDR", ""),
		From:          envString("NEXUSIM_IDENTITY_CHALLENGE_SMTP_FROM", ""),
		Username:      os.Getenv("NEXUSIM_IDENTITY_CHALLENGE_SMTP_USERNAME"),
		Password:      os.Getenv("NEXUSIM_IDENTITY_CHALLENGE_SMTP_PASSWORD"),
		ServerName:    envString("NEXUSIM_IDENTITY_CHALLENGE_SMTP_SERVER_NAME", ""),
		TLSMode:       envString("NEXUSIM_IDENTITY_CHALLENGE_SMTP_TLS_MODE", "starttls"),
		SubjectPrefix: envString("NEXUSIM_IDENTITY_CHALLENGE_SMTP_SUBJECT_PREFIX", "NexusIM"),
		SubjectTemplates: challengeSMTPTemplatesFromEnv(
			"NEXUSIM_IDENTITY_CHALLENGE_SMTP_SUBJECT_TEMPLATE",
		),
		BodyTemplates: challengeSMTPTemplatesFromEnv(
			"NEXUSIM_IDENTITY_CHALLENGE_SMTP_BODY_TEMPLATE",
		),
		Timeout: envDuration("NEXUSIM_IDENTITY_CHALLENGE_SMTP_TIMEOUT", 10*time.Second),
	})
}

func challengeSMTPTemplatesFromEnv(baseName string) map[types.ChallengeType]string {
	templates := make(map[types.ChallengeType]string)
	add := func(envName string, challengeType types.ChallengeType) {
		if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
			templates[challengeType] = value
		}
	}
	add(baseName, types.ChallengeType(""))
	add(baseName+"_EMAIL_VERIFICATION", types.ChallengeTypeEmailVerification)
	add(baseName+"_PASSWORD_RESET", types.ChallengeTypePasswordReset)
	if len(templates) == 0 {
		return nil
	}
	return templates
}

func challengeDeliveryMode() string {
	mode := strings.ToLower(strings.TrimSpace(envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_MODE", "noop")))
	if mode == "" {
		return "noop"
	}
	return mode
}

func newChallengeDeliveryWorkerNotifier() (app.ChallengeNotifier, string, error) {
	provider := strings.ToLower(strings.TrimSpace(envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_WORKER_PROVIDER", "webhook")))
	switch provider {
	case "webhook":
		notifier, err := newChallengeWebhookNotifier()
		return notifier, provider, err
	case "smtp":
		notifier, err := newChallengeSMTPNotifier()
		return notifier, provider, err
	default:
		return nil, provider, errors.New("unsupported NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_WORKER_PROVIDER")
	}
}

func newChallengeDeliveryTokenManager() (*tokeninfra.ChallengeDeliveryTokenManager, error) {
	if keyRing, ok, err := loadSecretKeyRingConfig(
		"NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEYRING_JSON",
		"NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEYRING_FILE",
	); err != nil {
		return nil, err
	} else if ok {
		return tokeninfra.NewChallengeDeliveryTokenManagerWithKeyRing(keyRing.Current, keyRing.Keys)
	}
	return tokeninfra.NewChallengeDeliveryTokenManager(envString(
		"NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEY",
		envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_SECRET", ""),
	))
}
