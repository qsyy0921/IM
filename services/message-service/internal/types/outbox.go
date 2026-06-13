package types

import "time"

const (
	OutboxStatusPending   = "PENDING"
	OutboxStatusPublished = "PUBLISHED"
	OutboxStatusDLQ       = "DLQ"
)

type OutboxMessage struct {
	ID                  int64
	EventID             EventID
	TenantID            TenantID
	ConversationID      ConversationID
	AggregateVersion    int64
	EventType           TimelineEventType
	EventVersion        string
	PartitionKey        string
	MappingVersion      string
	CorrelationID       string
	CausationID         string
	Producer            string
	PayloadJSON         []byte
	TraceID             string
	RetryCount          int
	FanoutMode          FanoutMode
	FanoutPolicyVersion int64
	PermissionVersion   int64
	Classification      string
	OccurredAt          time.Time
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
