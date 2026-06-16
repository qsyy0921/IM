package types

import "time"

const (
	OutboxStatusPending   = "PENDING"
	OutboxStatusPublished = "PUBLISHED"
	OutboxStatusDLQ       = "DLQ"

	ContactEventRequestCreated          = "contact.request.created.v1"
	ContactEventRequestAccepted         = "contact.request.accepted.v1"
	ContactEventRequestDeclined         = "contact.request.declined.v1"
	ContactEventRequestCanceled         = "contact.request.canceled.v1"
	ContactEventEdgeDeleted             = "contact.edge.deleted.v1"
	ContactEventEdgeBlocked             = "contact.edge.blocked.v1"
	ContactEventEdgeUnblocked           = "contact.edge.unblocked.v1"
	ContactEventRemarkUpdated           = "contact.edge.remark_updated.v1"
	ContactEventGroupUpdated            = "contact.edge.group_updated.v1"
	ContactEventPrivacyUpdated          = "contact.privacy.updated.v1"
	ContactEventPrivacyExceptionUpdated = "contact.privacy_exception.updated.v1"
	ContactEventPrivacyExceptionDeleted = "contact.privacy_exception.deleted.v1"
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

type OutboxRepairStats struct {
	Requested int
	Repaired  int
	Skipped   int
}

type KafkaPublishRecord struct {
	Key   []byte
	Value []byte
}
