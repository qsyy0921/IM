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
	err = tx.QueryRow(ctx, `
SELECT status, requires_approval
FROM agent_proposals
WHERE tenant_id = $1 AND proposal_id = $2
FOR UPDATE`,
		string(command.AuthContext.TenantID),
		command.NormalizedProposalID(),
	).Scan(&status, &requiresApproval)
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
