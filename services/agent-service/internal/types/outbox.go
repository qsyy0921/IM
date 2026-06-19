package types

import "time"

const (
	OutboxStatusPending   = "PENDING"
	OutboxStatusPublished = "PUBLISHED"
	OutboxStatusDLQ       = "DLQ"

	AgentEventProposalApproved = "agent.proposal.approved.v1"
)

type OutboxMessage struct {
	ID              int64
	EventID         string
	TenantID        TenantID
	ProposalID      string
	ApprovalID      string
	PreparedAuditID string
	SkillID         string
	ToolName        string
	ResourceType    string
	ResourceID      string
	RiskLevel       string
	EventType       string
	EventVersion    string
	PartitionKey    string
	MappingVersion  int64
	CorrelationID   string
	CausationID     string
	Producer        string
	PayloadJSON     []byte
	TraceID         string
	RetryCount      int
	OccurredAt      time.Time
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
