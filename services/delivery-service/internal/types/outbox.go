package types

import "time"

const (
	OutboxStatusPending   = "PENDING"
	OutboxStatusPublished = "PUBLISHED"
	OutboxStatusDLQ       = "DLQ"

	DeliveryEventInboxItemCreated = "delivery.inbox_item.created.v1"
	DeliveryEventAckRecorded      = "delivery.ack.recorded.v1"
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
	Audited   int
	Mutated   int
	Skipped   int
}

const (
	OutboxRepairModeAudit             = "audit"
	OutboxRepairModeRedriveDLQPending = "redrive-dlq-pending"
)

type OutboxRepairOptions struct {
	OutboxIDs []int64
	Mode      string
	Operator  string
	Reason    string
	DryRun    bool
}

type KafkaPublishRecord struct {
	Key   []byte
	Value []byte
}
