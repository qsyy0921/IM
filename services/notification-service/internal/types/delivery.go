package types

import "time"

const (
	AttemptStatusSending   = "SENDING"
	AttemptStatusSucceeded = "SUCCEEDED"
	AttemptStatusFailed    = "FAILED"

	FailureClassProviderUnavailable = "PROVIDER_UNAVAILABLE"
	FailureClassExpired             = "EXPIRED"

	PublicErrorProviderUnavailable = "notification provider unavailable"
	PublicErrorExpired             = "notification request expired"
)

type DeliveryRequest struct {
	NotificationRequest
	AttemptNumber          int
	ProviderID             string
	ProviderIdempotencyKey string
}

type DeliveryResult struct {
	ProviderID            string
	ProviderMessageIDHash string
}

type DeliveryFailure struct {
	FailureClass string
	PublicError  string
	RetryAfter   time.Time
	Permanent    bool
}

type DeliveryWorkerStats struct {
	Claimed      int
	Succeeded    int
	Retried      int
	DeadLettered int
}

func NewProviderUnavailableFailure() DeliveryFailure {
	return DeliveryFailure{
		FailureClass: FailureClassProviderUnavailable,
		PublicError:  PublicErrorProviderUnavailable,
	}
}

func NewExpiredDeliveryFailure() DeliveryFailure {
	return DeliveryFailure{
		FailureClass: FailureClassExpired,
		PublicError:  PublicErrorExpired,
		Permanent:    true,
	}
}
