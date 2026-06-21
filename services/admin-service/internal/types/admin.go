package types

import "time"

const (
	OperationStatusSubmitted             = "SUBMITTED"
	OperationStatusApproved              = "APPROVED"
	OperationStatusRejected              = "REJECTED"
	OperationStatusExecuting             = "EXECUTING"
	OperationStatusSucceeded             = "SUCCEEDED"
	OperationStatusFailed                = "FAILED"
	OperationStatusCanceled              = "CANCELED"
	OperationStatusCompensationRequested = "COMPENSATION_REQUESTED"

	DecisionApprove = "APPROVE"
	DecisionReject  = "REJECT"

	RiskLevelLow      = "LOW"
	RiskLevelMedium   = "MEDIUM"
	RiskLevelHigh     = "HIGH"
	RiskLevelCritical = "CRITICAL"

	AdminEventOperationSubmitted             = "admin.operation.submitted.v1"
	AdminEventOperationApproved              = "admin.operation.approved.v1"
	AdminEventOperationRejected              = "admin.operation.rejected.v1"
	AdminEventOperationExecuted              = "admin.operation.executed.v1"
	AdminEventOperationFailed                = "admin.operation.failed.v1"
	AdminEventOperationCompensationRequested = "admin.operation.compensation_requested.v1"

	OutboxStatusPending   = "PENDING"
	OutboxStatusPublished = "PUBLISHED"
	OutboxStatusDLQ       = "DLQ"
)

type CreateAdminOperationCommand struct {
	AuthContext          AuthContext
	OperatorRef          string
	OperatorRole         string
	OperationType        string
	TargetRefHash        string
	RiskLevel            string
	PayloadSchemaVersion string
	OperationPayloadJSON string
	ReasonRef            string
	EvidenceRefs         []string
	IdempotencyKey       string
	CorrelationID        string
	CausationID          string
	TraceID              string
}

type ApproveAdminOperationCommand struct {
	AuthContext       AuthContext
	OperationID       string
	ApproverRef       string
	ApproverRole      string
	Decision          string
	ApprovalPolicyRef string
	ReasonRef         string
	EvidenceRefs      []string
	IdempotencyKey    string
	CorrelationID     string
	CausationID       string
	TraceID           string
}

type GetAdminOperationCommand struct {
	AuthContext AuthContext
	OperationID string
}

type ListAdminOperationsCommand struct {
	AuthContext   AuthContext
	Status        string
	OperationType string
	PageSize      int
}

type RequestAdminOperationCompensationCommand struct {
	TenantID              TenantID
	OperationID           string
	RequestedBy           string
	CompensationReasonRef string
	DryRun                bool
}

type AdminOperation struct {
	TenantID             TenantID
	OperationID          string
	IdempotencyKey       string
	CommandHash          string
	OperationType        string
	TargetRefHash        string
	RiskLevel            string
	PayloadSchemaVersion string
	PayloadJSON          string
	PayloadHash          string
	ReasonRef            string
	EvidenceRefs         []string
	Status               string
	RequestedBy          string
	RequestedAt          time.Time
	ApprovedBy           string
	ApprovedAt           time.Time
	CorrelationID        string
	CausationID          string
	TraceID              string
	UpdatedAt            time.Time
}

type AdminApproval struct {
	TenantID          TenantID
	ApprovalID        string
	OperationID       string
	IdempotencyKey    string
	CommandHash       string
	ApproverRef       string
	Decision          string
	ApprovalPolicyRef string
	ReasonRef         string
	EvidenceRefs      []string
	CreatedAt         time.Time
}

type AdminOperationResult struct {
	TenantID             TenantID
	ResultID             string
	OperationID          string
	DownstreamService    string
	DownstreamRequestRef string
	Status               string
	FailureClass         string
	PublicError          string
	CreatedAt            time.Time
	CompletedAt          time.Time
}

type OperationExecutionResult struct {
	DownstreamService    string
	DownstreamRequestRef string
	Status               string
	FailureClass         string
	PublicError          string
}

type OperationWorkerStats struct {
	Claimed   int
	Succeeded int
	Failed    int
}

type OutboxMessage struct {
	EventID          string
	TenantID         TenantID
	OperationID      string
	EventType        string
	EventVersion     int
	PartitionKey     string
	Producer         string
	PayloadJSON      []byte
	RetryCount       int
	OccurredAt       time.Time
	AggregateVersion int64
	CorrelationID    string
	CausationID      string
	TraceID          string
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
