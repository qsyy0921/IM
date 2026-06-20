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
