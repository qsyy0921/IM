package types

import "time"

const (
	OutboxStatusPending   = "PENDING"
	OutboxStatusPublished = "PUBLISHED"
	OutboxStatusDLQ       = "DLQ"

	ReceiptEventMessageReceived = "receipt.message.received.v1"
	ReceiptEventMessageRead     = "receipt.message.read.v1"
)

type OutboxMessage struct {
	ID               int64
	EventID          string
	TenantID         TenantID
	ConversationID   ConversationID
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

type OutboxRepairStats struct {
	Requested int
	Repaired  int
	Skipped   int
}

type KafkaPublishRecord struct {
	Key   []byte
	Value []byte
}
