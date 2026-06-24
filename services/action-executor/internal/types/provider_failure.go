package types

import "time"

type ProviderFailureAuditOptions struct {
	TenantID string
	Status   string
	ToolName string
	Limit    int
}

type ProviderFailureAuditRow struct {
	TenantID          string
	ProviderFailureID string
	ExecutionID       string
	ResultID          string
	ProposalID        string
	ApprovalID        string
	PreparedAuditID   string
	UserID            string
	SkillID           string
	ToolName          string
	ResourceType      string
	ResourceID        string
	Classification    string
	Status            string
	Retryable         bool
	RetryCount        int
	NextRetryAt       *time.Time
	DeadLetteredAt    *time.Time
	FailureRef        string
	CreatedAt         time.Time
}
