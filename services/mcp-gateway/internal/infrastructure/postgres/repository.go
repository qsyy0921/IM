package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/mcp-gateway/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return Repository{pool: pool}
}

func (repository Repository) InsertToolCallAudit(ctx context.Context, audit types.ToolCallAudit) error {
	if repository.pool == nil {
		return errors.Join(types.ErrAuditWriteFailed, errors.New("nil pg pool"))
	}
	_, err := repository.pool.Exec(ctx, `
INSERT INTO mcp_gateway_tool_call_audits (
    tenant_id,
    audit_id,
    user_id,
    device_id,
    session_id,
    trace_id,
    request_id,
    skill_id,
    tool_name,
    tool_action,
    resource_type,
    resource_id,
    risk_level,
    intent,
    idempotency_key,
    input_sha256,
    allowed,
    requires_approval,
    permission_version,
    classification,
    reason,
    decision_source,
    status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19, $20, $21,
    $22, $23
)`,
		string(audit.TenantID),
		audit.AuditID,
		string(audit.UserID),
		audit.DeviceID,
		audit.SessionID,
		audit.TraceID,
		audit.RequestID,
		audit.SkillID,
		audit.ToolName,
		audit.Action,
		audit.ResourceType,
		audit.ResourceID,
		audit.RiskLevel,
		audit.Intent,
		audit.IdempotencyKey,
		audit.InputSHA256,
		audit.Allowed,
		audit.RequiresApproval,
		audit.PermissionVersion,
		audit.Classification,
		truncateLowSensitive(audit.Reason, 512),
		audit.DecisionSource,
		audit.Status,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrAuditWriteFailed, err)
	}
	return nil
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
