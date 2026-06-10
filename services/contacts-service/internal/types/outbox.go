package types

import "time"

const (
	OutboxStatusPending   = "PENDING"
	OutboxStatusPublished = "PUBLISHED"
	OutboxStatusDLQ       = "DLQ"

	ContactEventRequestCreated  = "contact.request.created.v1"
	ContactEventRequestAccepted = "contact.request.accepted.v1"
	ContactEventRequestDeclined = "contact.request.declined.v1"
)

type OutboxMessage struct {
	ID               int64
	EventID          string
	TenantID         TenantID
	AggregateType    string
	AggregateID      string
	AggregateVersion int64
	EventType        string
	EventVersion     string
	PartitionKey     string
	MappingVersion   int64
	CorrelationID    string
	CausationID      string
	Producer         string
	PayloadJSON      []byte
	TraceID          string
	RetryCount       int
	OccurredAt       time.Time
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
