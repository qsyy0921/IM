package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

const (
	agentApprovalOutboxEventType      = "agent.proposal.approved.v1"
	agentApprovalOutboxEventVersion   = "v1"
	agentApprovalOutboxMappingVersion = 1
	agentApprovalOutboxProducer       = "agent-service"
)

func NewRepository(pool *pgxpool.Pool) Repository {
	return Repository{pool: pool}
}

func (repository Repository) StoreAgentProposal(ctx context.Context, proposal types.StoredAgentProposal) error {
	if repository.pool == nil {
		return errors.Join(types.ErrProposalStoreUnavailable, errors.New("nil pg pool"))
	}
	citationsJSON := strings.TrimSpace(proposal.CitationsJSON)
	if citationsJSON == "" {
		citationsJSON = "[]"
	}
	_, err := repository.pool.Exec(ctx, `
INSERT INTO agent_proposals (
    tenant_id,
    proposal_id,
    user_id,
    conversation_id,
    objective,
    skill_id,
    prepared_audit_id,
    tool_name,
    tool_action,
    resource_type,
    resource_id,
    risk_level,
    intent,
    status,
    proposal_text,
    requires_approval,
    allowed,
    permission_version,
    classification,
    reason,
    decision_source,
    evidence_pack_id,
    citations_json,
    agent_version,
    generated_by_llm
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19, $20, $21,
    $22, $23::jsonb, $24, $25
) ON CONFLICT (tenant_id, proposal_id) DO NOTHING`,
		string(proposal.TenantID),
		proposal.ProposalID,
		string(proposal.UserID),
		string(proposal.ConversationID),
		truncateLowSensitive(proposal.Objective, 2048),
		proposal.SkillID,
		proposal.PreparedAuditID,
		proposal.ToolName,
		proposal.ToolAction,
		proposal.ResourceType,
		proposal.ResourceID,
		proposal.RiskLevel,
		truncateLowSensitive(proposal.Intent, 2048),
		proposal.Status,
		truncateLowSensitive(proposal.ProposalText, 8192),
		proposal.RequiresApproval,
		proposal.Allowed,
		proposal.PermissionVersion,
		truncateLowSensitive(proposal.Classification, 128),
		truncateLowSensitive(proposal.Reason, 512),
		truncateLowSensitive(proposal.DecisionSource, 128),
		proposal.EvidencePackID,
		citationsJSON,
		proposal.AgentVersion,
		proposal.GeneratedByLLM,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrProposalStoreUnavailable, err)
	}
	return nil
}

func (repository Repository) ApproveAgentProposal(
	ctx context.Context,
	command types.ApproveAgentProposalCommand,
	approvalID string,
) (types.ApproveAgentProposalResult, error) {
	if repository.pool == nil {
		return types.ApproveAgentProposalResult{}, errors.Join(types.ErrProposalStoreUnavailable, errors.New("nil pg pool"))
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return types.ApproveAgentProposalResult{}, fmt.Errorf("%w: %v", types.ErrProposalStoreUnavailable, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	var requiresApproval bool
	var preparedAuditID string
	var skillID string
	var toolName string
	var resourceType string
	var resourceID string
	var riskLevel string
	err = tx.QueryRow(ctx, `
SELECT
    status,
    requires_approval,
    prepared_audit_id,
    skill_id,
    tool_name,
    resource_type,
    resource_id,
    risk_level
FROM agent_proposals
WHERE tenant_id = $1 AND proposal_id = $2
FOR UPDATE`,
		string(command.AuthContext.TenantID),
		command.NormalizedProposalID(),
	).Scan(
		&status,
		&requiresApproval,
		&preparedAuditID,
		&skillID,
		&toolName,
		&resourceType,
		&resourceID,
		&riskLevel,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.ApproveAgentProposalResult{}, types.ErrProposalNotFound
	}
	if err != nil {
		return types.ApproveAgentProposalResult{}, fmt.Errorf("%w: %v", types.ErrProposalStoreUnavailable, err)
	}
	if status != types.AgentProposalStatusProposed || !requiresApproval {
		return types.ApproveAgentProposalResult{}, types.ErrProposalNotApprovable
	}
	approvedAt := time.Now().UTC()
	tag, err := tx.Exec(ctx, `
UPDATE agent_proposals
SET status = 'APPROVED',
    approval_id = $3,
    approved_by_user_id = $4,
    approved_at = $5,
    approval_reason = $6,
    updated_at = now()
WHERE tenant_id = $1 AND proposal_id = $2 AND status = 'PROPOSED'`,
		string(command.AuthContext.TenantID),
		command.NormalizedProposalID(),
		approvalID,
		string(command.AuthContext.UserID),
		approvedAt,
		truncateLowSensitive(command.NormalizedReason(), 512),
	)
	if err != nil {
		return types.ApproveAgentProposalResult{}, fmt.Errorf("%w: %v", types.ErrProposalStoreUnavailable, err)
	}
	if tag.RowsAffected() != 1 {
		return types.ApproveAgentProposalResult{}, types.ErrProposalNotApprovable
	}
	if err := insertApprovalOutbox(ctx, tx, approvalOutboxEvent{
		TenantID:          string(command.AuthContext.TenantID),
		ProposalID:        command.NormalizedProposalID(),
		ApprovalID:        approvalID,
		PreparedAuditID:   preparedAuditID,
		SkillID:           skillID,
		ToolName:          toolName,
		ResourceType:      resourceType,
		ResourceID:        resourceID,
		RiskLevel:         riskLevel,
		ApprovedByUserID:  string(command.AuthContext.UserID),
		ApprovedAtUnixMs:  approvedAt.UnixMilli(),
		ApprovalEventType: agentApprovalOutboxEventType,
	}); err != nil {
		return types.ApproveAgentProposalResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ApproveAgentProposalResult{}, fmt.Errorf("%w: %v", types.ErrProposalStoreUnavailable, err)
	}
	return types.ApproveAgentProposalResult{
		ProposalID:       command.NormalizedProposalID(),
		ApprovalID:       approvalID,
		Status:           types.AgentProposalStatusApproved,
		ApprovedByUserID: command.AuthContext.UserID,
		ApprovedAt:       approvedAt,
	}, nil
}

func (repository Repository) VerifyApprovedAgentProposal(
	ctx context.Context,
	command types.VerifyApprovedAgentProposalCommand,
) (types.VerifyApprovedAgentProposalResult, error) {
	if repository.pool == nil {
		return types.VerifyApprovedAgentProposalResult{}, errors.Join(types.ErrProposalStoreUnavailable, errors.New("nil pg pool"))
	}
	var row types.VerifyApprovedAgentProposalResult
	var userID string
	var conversationID string
	var approvedAt time.Time
	err := repository.pool.QueryRow(ctx, `
SELECT
    proposal_id,
    approval_id,
    status,
    user_id,
    conversation_id,
    skill_id,
    prepared_audit_id,
    tool_name,
    resource_type,
    resource_id,
    risk_level,
    COALESCE(approved_at, 'epoch'::timestamptz)
FROM agent_proposals
WHERE tenant_id = $1 AND proposal_id = $2`,
		string(command.AuthContext.TenantID),
		command.ProposalID,
	).Scan(
		&row.ProposalID,
		&row.ApprovalID,
		&row.Status,
		&userID,
		&conversationID,
		&row.SkillID,
		&row.PreparedAuditID,
		&row.ToolName,
		&row.ResourceType,
		&row.ResourceID,
		&row.RiskLevel,
		&approvedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.VerifyApprovedAgentProposalResult{}, types.ErrProposalNotFound
	}
	if err != nil {
		return types.VerifyApprovedAgentProposalResult{}, fmt.Errorf("%w: %v", types.ErrProposalStoreUnavailable, err)
	}
	row.UserID = types.UserID(userID)
	row.ConversationID = types.ConversationID(conversationID)
	row.ApprovedAt = approvedAt
	if row.Status != types.AgentProposalStatusApproved {
		return types.VerifyApprovedAgentProposalResult{}, types.ErrProposalNotApproved
	}
	if row.UserID != command.AuthContext.UserID ||
		row.ApprovalID != command.ApprovalID ||
		row.PreparedAuditID != command.PreparedAuditID ||
		row.SkillID != command.SkillID ||
		row.ToolName != command.ToolName ||
		row.ResourceType != command.ResourceType ||
		row.ResourceID != command.ResourceID {
		return types.VerifyApprovedAgentProposalResult{}, types.ErrProposalMismatch
	}
	return row, nil
}

type approvalOutboxEvent struct {
	TenantID          string
	ProposalID        string
	ApprovalID        string
	PreparedAuditID   string
	SkillID           string
	ToolName          string
	ResourceType      string
	ResourceID        string
	RiskLevel         string
	ApprovedByUserID  string
	ApprovedAtUnixMs  int64
	ApprovalEventType string
}

func insertApprovalOutbox(ctx context.Context, tx pgx.Tx, event approvalOutboxEvent) error {
	_, err := tx.Exec(ctx, `
INSERT INTO agent_approval_outbox (
    event_id,
    tenant_id,
    proposal_id,
    approval_id,
    prepared_audit_id,
    skill_id,
    tool_name,
    resource_type,
    resource_id,
    risk_level,
    event_type,
    event_version,
    mapping_version,
    partition_key,
    producer,
    payload_json
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12, $13, $14, $15,
    jsonb_build_object(
        'schema_version', 1,
        'event_type', $11::text,
        'tenant_id', $2::text,
        'proposal_id', $3::text,
        'approval_id', $4::text,
        'prepared_audit_id', $5::text,
        'skill_id', $6::text,
        'tool_name', $7::text,
        'resource_type', $8::text,
        'resource_id', $9::text,
        'risk_level', $10::text,
        'approved_by_user_id', $16::text,
        'approved_at_unix_ms', $17::bigint
    )
) ON CONFLICT (tenant_id, approval_id) DO NOTHING`,
		approvalEventID(event.ApprovalID),
		event.TenantID,
		event.ProposalID,
		event.ApprovalID,
		event.PreparedAuditID,
		event.SkillID,
		event.ToolName,
		event.ResourceType,
		event.ResourceID,
		event.RiskLevel,
		event.ApprovalEventType,
		agentApprovalOutboxEventVersion,
		agentApprovalOutboxMappingVersion,
		event.ProposalID,
		agentApprovalOutboxProducer,
		event.ApprovedByUserID,
		event.ApprovedAtUnixMs,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrProposalStoreUnavailable, err)
	}
	return nil
}

func approvalEventID(approvalID string) string {
	return "agent_approval_" + strings.TrimSpace(approvalID)
}

func truncateLowSensitive(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
