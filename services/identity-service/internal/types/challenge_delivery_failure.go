package types

import (
	"context"
	"errors"
	"strings"
)

const (
	ChallengeDeliveryFailureClassConfiguration      = "configuration"
	ChallengeDeliveryFailureClassProviderNonSuccess = "provider_non_success"
	ChallengeDeliveryFailureClassTimeout            = "timeout"
	ChallengeDeliveryFailureClassNetwork            = "network"
	ChallengeDeliveryFailureClassSerialization      = "serialization"
	ChallengeDeliveryFailureClassTokenCrypto        = "token_crypto"
	ChallengeDeliveryFailureClassDeliveryFailed     = "delivery_failed"
	ChallengeDeliveryFailureClassCanceled           = "canceled"
	ChallengeDeliveryFailureClassInactive           = "inactive"
	ChallengeDeliveryFailureClassUnknown            = "unknown"
)

func ClassifyChallengeDeliveryFailure(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return ChallengeDeliveryFailureClassCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ChallengeDeliveryFailureClassTimeout
	}
	return ClassifyChallengeDeliveryFailureMessage(err.Error(), errors.Is(err, ErrChallengeDeliveryFailed))
}

func ClassifyChallengeDeliveryFailureMessage(message string, isChallengeDeliveryFailure bool) string {
	message = strings.ToLower(strings.TrimSpace(message))
	switch {
	case message == "":
		return ChallengeDeliveryFailureClassUnknown
	case strings.Contains(message, "challenge no longer active before delivery"):
		return ChallengeDeliveryFailureClassInactive
	case strings.Contains(message, "not configured") ||
		strings.Contains(message, "url is required") ||
		strings.Contains(message, "key is required") ||
		strings.Contains(message, "encryption key is required"):
		return ChallengeDeliveryFailureClassConfiguration
	case strings.Contains(message, "non-success status"):
		return ChallengeDeliveryFailureClassProviderNonSuccess
	case strings.Contains(message, "timeout") ||
		strings.Contains(message, "deadline exceeded"):
		return ChallengeDeliveryFailureClassTimeout
	case strings.Contains(message, "connection refused") ||
		strings.Contains(message, "no such host") ||
		strings.Contains(message, "network") ||
		strings.Contains(message, "temporary failure"):
		return ChallengeDeliveryFailureClassNetwork
	case strings.Contains(message, "marshal") ||
		strings.Contains(message, "json"):
		return ChallengeDeliveryFailureClassSerialization
	case strings.Contains(message, "decrypt") ||
		strings.Contains(message, "encrypt") ||
		strings.Contains(message, "ciphertext") ||
		strings.Contains(message, "nonce"):
		return ChallengeDeliveryFailureClassTokenCrypto
	case strings.Contains(message, "context canceled") ||
		strings.Contains(message, "cancelled") ||
		strings.Contains(message, "canceled"):
		return ChallengeDeliveryFailureClassCanceled
	case isChallengeDeliveryFailure:
		return ChallengeDeliveryFailureClassDeliveryFailed
	default:
		return ChallengeDeliveryFailureClassUnknown
	}
}
