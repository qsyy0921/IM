package types

import "time"

const (
	OutboxStatusPending   = "PENDING"
	OutboxStatusPublished = "PUBLISHED"
	OutboxStatusDLQ       = "DLQ"

	NotificationEventRequestAccepted      = "notification.request.accepted.v1"
	NotificationEventDeliverySucceeded    = "notification.delivery.succeeded.v1"
	NotificationEventDeliveryFailed       = "notification.delivery.failed.v1"
	NotificationEventDeliveryDeadLettered = "notification.delivery.dead_lettered.v1"
	NotificationEventRecipientSuppressed  = "notification.recipient.suppressed.v1"
)

type OutboxMessage struct {
	EventID          string
	TenantID         TenantID
	RequestID        string
	EventType        string
	EventVersion     int32
	PartitionKey     string
	Producer         string
	PayloadJSON      []byte
	RetryCount       int
	OccurredAt       time.Time
	TraceID          string
	CorrelationID    string
	CausationID      string
	AggregateVersion int64
}

type OutboxRelayStats struct {
	Fetched      int
	Published    int
	Retried      int
	DeadLettered int
}

type KafkaPublishRecord struct {
	Key   []byte
	Value []byte
}
