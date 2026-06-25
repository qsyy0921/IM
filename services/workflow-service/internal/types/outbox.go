package types

import "time"

const (
	OutboxStatusPending   = "PENDING"
	OutboxStatusPublished = "PUBLISHED"
	OutboxStatusDLQ       = "DLQ"
)

type OutboxMessage struct {
	EventID       string
	TenantID      TenantID
	WorkflowID    string
	AggregateType string
	AggregateID   string
	EventType     string
	EventVersion  int32
	PartitionKey  string
	Producer      string
	PayloadJSON   []byte
	RetryCount    int
	OccurredAt    time.Time
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
