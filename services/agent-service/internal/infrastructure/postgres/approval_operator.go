package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

type AgentProposalApprovalAuditOptions struct {
	TenantID     string
	ProposalID   string
	UserID       string
	Status       string
	ToolName     string
	ResourceType string
	Limit        int
}

type AgentProposalApprovalAuditRow struct {
	TenantID              string
	ProposalID            string
	UserID                string
	ConversationID        string
	SkillID               string
	PreparedAuditID       string
	ToolName              string
	ResourceType          string
	ResourceID            string
	RiskLevel             string
	Status                string
	RequiresApproval      bool
	Allowed               bool
	PermissionVersion     int64
	Classification        string
	DecisionSource        string
	EvidencePackID        string
	GeneratedByLLM        bool
	ApprovalID            string
	ApprovedByUserID      string
	ApprovalReasonPresent bool
	ApprovedAt            time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (repository Repository) AuditAgentProposalApprovals(
	ctx context.Context,
	options AgentProposalApprovalAuditOptions,
) ([]AgentProposalApprovalAuditRow, error) {
	if repository.pool == nil {
		return nil, errors.Join(types.ErrProposalStoreUnavailable, errors.New("nil pg pool"))
	}
	limit := options.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := repository.pool.Query(ctx, `
SELECT
    tenant_id,
    proposal_id,
    user_id,
    conversation_id,
    skill_id,
    prepared_audit_id,
    tool_name,
    resource_type,
    resource_id,
    risk_level,
    status,
    requires_approval,
    allowed,
    permission_version,
    classification,
    decision_source,
    evidence_pack_id,
    generated_by_llm,
    approval_id,
    approved_by_user_id,
    approval_reason <> '',
    COALESCE(approved_at, 'epoch'::timestamptz),
    created_at,
    updated_at
FROM agent_proposals
WHERE ($1 = '' OR tenant_id = $1)
  AND ($2 = '' OR proposal_id = $2)
  AND ($3 = '' OR user_id = $3)
  AND ($4 = '' OR status = $4)
  AND ($5 = '' OR tool_name = $5)
  AND ($6 = '' OR resource_type = $6)
ORDER BY created_at DESC
LIMIT $7`,
		strings.TrimSpace(options.TenantID),
		strings.TrimSpace(options.ProposalID),
		strings.TrimSpace(options.UserID),
		strings.TrimSpace(options.Status),
		strings.TrimSpace(options.ToolName),
		strings.TrimSpace(options.ResourceType),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrProposalStoreUnavailable, err)
	}
	defer rows.Close()

	result := make([]AgentProposalApprovalAuditRow, 0)
	for rows.Next() {
		var row AgentProposalApprovalAuditRow
		if err := rows.Scan(
			&row.TenantID,
			&row.ProposalID,
			&row.UserID,
			&row.ConversationID,
			&row.SkillID,
			&row.PreparedAuditID,
			&row.ToolName,
			&row.ResourceType,
			&row.ResourceID,
			&row.RiskLevel,
			&row.Status,
			&row.RequiresApproval,
			&row.Allowed,
			&row.PermissionVersion,
			&row.Classification,
			&row.DecisionSource,
			&row.EvidencePackID,
			&row.GeneratedByLLM,
			&row.ApprovalID,
			&row.ApprovedByUserID,
			&row.ApprovalReasonPresent,
			&row.ApprovedAt,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("%w: %v", types.ErrProposalStoreUnavailable, err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", types.ErrProposalStoreUnavailable, err)
	}
	return result, nil
}

func (repository Repository) GetAgentProposalApprovalCandidate(
	ctx context.Context,
	tenantID string,
	proposalID string,
) (AgentProposalApprovalAuditRow, error) {
	rows, err := repository.AuditAgentProposalApprovals(ctx, AgentProposalApprovalAuditOptions{
		TenantID:   tenantID,
		ProposalID: proposalID,
		Limit:      1,
	})
	if err != nil {
		return AgentProposalApprovalAuditRow{}, err
	}
	if len(rows) == 0 {
		return AgentProposalApprovalAuditRow{}, types.ErrProposalNotFound
	}
	return rows[0], nil
}
