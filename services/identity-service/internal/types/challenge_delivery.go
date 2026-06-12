package types

import "time"

const (
	ChallengeDeliveryStatusPending   = "PENDING"
	ChallengeDeliveryStatusDelivered = "DELIVERED"
	ChallengeDeliveryStatusDLQ       = "DLQ"
	ChallengeDeliveryStatusCanceled  = "CANCELED"
)

type ChallengeDeliveryMessage struct {
	ID             int64
	TenantID       TenantID
	UserID         UserID
	ChallengeID    ChallengeID
	Type           ChallengeType
	Channel        VerificationChannel
	Destination    string
	EncryptedToken EncryptedChallengeToken
	ExpiresAt      time.Time
	TraceID        string
	RequestID      string
	RetryCount     int
	CreatedAt      time.Time
}

type ChallengeDeliveryStats struct {
	Fetched      int
	Delivered    int
	Retried      int
	DeadLettered int
	Canceled     int
}
