package main

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/app"
	mfainfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/mfa"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func newMFASecretManager() (app.MFASecretManager, error) {
	if keyRing, ok, err := loadSecretKeyRingConfig(
		"NEXUSIM_IDENTITY_MFA_SECRET_KEYRING_JSON",
		"NEXUSIM_IDENTITY_MFA_SECRET_KEYRING_FILE",
	); err != nil {
		return nil, err
	} else if ok {
		return mfainfra.NewTOTPManagerWithKeyRing(keyRing.Current, keyRing.Keys)
	}
	secret := envString("NEXUSIM_IDENTITY_MFA_SECRET_KEY", "")
	if secret == "" {
		return disabledMFASecretManager{}, nil
	}
	return mfainfra.NewTOTPManager(secret)
}

type secretKeyRingConfig struct {
	Current string            `json:"current"`
	Keys    map[string]string `json:"keys"`
}

func loadSecretKeyRingConfig(jsonEnv string, fileEnv string) (secretKeyRingConfig, bool, error) {
	raw := strings.TrimSpace(os.Getenv(jsonEnv))
	if raw == "" {
		path := strings.TrimSpace(os.Getenv(fileEnv))
		if path == "" {
			return secretKeyRingConfig{}, false, nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return secretKeyRingConfig{}, true, err
		}
		raw = strings.TrimSpace(string(content))
		if raw == "" {
			return secretKeyRingConfig{}, true, errors.New("identity secret keyring file is empty")
		}
	}
	if raw == "" {
		return secretKeyRingConfig{}, false, nil
	}
	var config secretKeyRingConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return secretKeyRingConfig{}, true, err
	}
	config.Current = strings.TrimSpace(config.Current)
	if config.Current == "" {
		return secretKeyRingConfig{}, true, errors.New("identity secret keyring current key version is required")
	}
	if len(config.Keys) == 0 {
		return secretKeyRingConfig{}, true, errors.New("identity secret keyring keys are required")
	}
	normalized := make(map[string]string, len(config.Keys))
	for keyVersion, secret := range config.Keys {
		keyVersion = strings.TrimSpace(keyVersion)
		if keyVersion == "" {
			return secretKeyRingConfig{}, true, errors.New("identity secret keyring key version is required")
		}
		if _, ok := normalized[keyVersion]; ok {
			return secretKeyRingConfig{}, true, errors.New("identity secret keyring duplicate key version")
		}
		if strings.TrimSpace(secret) == "" {
			return secretKeyRingConfig{}, true, errors.New("identity secret keyring key value is required")
		}
		normalized[keyVersion] = secret
	}
	if _, ok := normalized[config.Current]; !ok {
		return secretKeyRingConfig{}, true, errors.New("identity secret keyring current key version is not configured")
	}
	config.Keys = normalized
	return config, true, nil
}

func newMFARecoveryCodeManager() (app.MFARecoveryCodeManager, error) {
	secret := envString("NEXUSIM_IDENTITY_MFA_RECOVERY_CODE_SECRET", "")
	if secret == "" {
		return disabledMFARecoveryCodeManager{}, nil
	}
	return mfainfra.NewRecoveryCodeManager(secret)
}

func identityMFARiskPolicyFromEnv() app.LoginRiskPolicy {
	return app.LoginRiskPolicy{
		MaxFailedAttempts: envInt("NEXUSIM_IDENTITY_MFA_MAX_FAILED_ATTEMPTS", app.DefaultMFAMaxFailedAttempts),
		FailureWindow:     envDuration("NEXUSIM_IDENTITY_MFA_FAILURE_WINDOW", app.DefaultMFAFailureWindow),
		LockDuration:      envDuration("NEXUSIM_IDENTITY_MFA_LOCK_DURATION", app.DefaultMFALockDuration),
	}
}

func identityMFARecoveryRiskPolicyFromEnv() app.LoginRiskPolicy {
	return app.LoginRiskPolicy{
		MaxFailedAttempts: envInt("NEXUSIM_IDENTITY_MFA_RECOVERY_MAX_FAILED_ATTEMPTS", app.DefaultMFAMaxFailedAttempts),
		FailureWindow:     envDuration("NEXUSIM_IDENTITY_MFA_RECOVERY_FAILURE_WINDOW", app.DefaultMFAFailureWindow),
		LockDuration:      envDuration("NEXUSIM_IDENTITY_MFA_RECOVERY_LOCK_DURATION", app.DefaultMFALockDuration),
	}
}

type disabledMFASecretManager struct{}

func (disabledMFASecretManager) NewTOTPSecret() (string, types.EncryptedMFASecret, error) {
	return "", types.EncryptedMFASecret{}, types.NewMFAUnavailable("mfa secret encryption key is required")
}

func (disabledMFASecretManager) VerifyTOTP(types.EncryptedMFASecret, string, time.Time) (bool, error) {
	return false, types.NewMFAUnavailable("mfa secret encryption key is required")
}

func (disabledMFASecretManager) OTPAuthURI(string, string, string) string {
	return ""
}

type disabledMFARecoveryCodeManager struct{}

func (disabledMFARecoveryCodeManager) NewRecoveryCodes(int) ([]types.MFARecoveryCode, error) {
	return nil, types.NewMFAUnavailable("mfa recovery code key is required")
}

func (disabledMFARecoveryCodeManager) HashRecoveryCode(string) (string, error) {
	return "", types.NewMFAUnavailable("mfa recovery code key is required")
}
