package mfa

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"strings"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

const (
	defaultRecoveryCodeCount = 10
	recoveryCodeBytes        = 10
	recoveryCodeIDBytes      = 16
	recoveryCodeHashPrefix   = "mfa-recovery-hmac-sha256"
)

type RecoveryCodeManager struct {
	key []byte
}

func NewRecoveryCodeManager(secret string) (*RecoveryCodeManager, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, types.NewMFAUnavailable("mfa recovery code key is required")
	}
	sum := sha256.Sum256([]byte(secret))
	return &RecoveryCodeManager{key: sum[:]}, nil
}

func (manager *RecoveryCodeManager) NewRecoveryCodes(count int) ([]types.MFARecoveryCode, error) {
	if manager == nil || len(manager.key) == 0 {
		return nil, types.NewMFAUnavailable("mfa recovery code key is required")
	}
	if count <= 0 {
		count = defaultRecoveryCodeCount
	}
	codes := make([]types.MFARecoveryCode, 0, count)
	for i := 0; i < count; i++ {
		code, err := randomRecoveryCode()
		if err != nil {
			return nil, err
		}
		codeID, err := randomRecoveryCodeID()
		if err != nil {
			return nil, err
		}
		codeHash, err := manager.HashRecoveryCode(code)
		if err != nil {
			return nil, err
		}
		codes = append(codes, types.MFARecoveryCode{CodeID: codeID, Code: code, CodeHash: codeHash})
	}
	return codes, nil
}

func (manager *RecoveryCodeManager) HashRecoveryCode(code string) (string, error) {
	if manager == nil || len(manager.key) == 0 {
		return "", types.NewMFAUnavailable("mfa recovery code key is required")
	}
	normalized := normalizeRecoveryCode(code)
	if normalized == "" {
		return "", types.NewInvalidMFA("invalid recovery code")
	}
	mac := hmac.New(sha256.New, manager.key)
	_, _ = mac.Write([]byte(normalized))
	return recoveryCodeHashPrefix + "$local-v1$" + hex.EncodeToString(mac.Sum(nil)), nil
}

func randomRecoveryCode() (string, error) {
	raw := make([]byte, recoveryCodeBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", types.NewMFAUnavailable(err.Error())
	}
	encoded := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw))
	return encoded[0:4] + "-" + encoded[4:8] + "-" + encoded[8:12] + "-" + encoded[12:16], nil
}

func randomRecoveryCodeID() (string, error) {
	raw := make([]byte, recoveryCodeIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", types.NewMFAUnavailable(err.Error())
	}
	return "mrc_" + hex.EncodeToString(raw), nil
}

func normalizeRecoveryCode(code string) string {
	code = strings.TrimSpace(strings.ToLower(code))
	var builder strings.Builder
	builder.Grow(len(code))
	for _, r := range code {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
