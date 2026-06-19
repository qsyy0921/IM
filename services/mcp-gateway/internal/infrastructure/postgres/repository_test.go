package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/mcp-gateway/internal/types"
)

func TestRepositoryInsertToolCallAuditIntegration(t *testing.T) {
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, migrationSQL); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM mcp_gateway_tool_call_audits WHERE tenant_id = 'tenant-mcp-test'`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	repository := NewRepository(pool)
	err = repository.InsertToolCallAudit(ctx, types.ToolCallAudit{
		TenantID:          "tenant-mcp-test",
		AuditID:           "audit-1",
		UserID:            "user-1",
		DeviceID:          "device-1",
		SkillID:           "skill-1",
		ToolName:          "conversation.note.create",
		Action:            types.ToolActionCall,
		ResourceType:      "conversation",
		ResourceID:        "conv-1",
		RiskLevel:         "LOW",
		Intent:            "draft note",
		IdempotencyKey:    "idem-1",
		InputSHA256:       strings.Repeat("a", 64),
		Allowed:           true,
		RequiresApproval:  true,
		PermissionVersion: 11,
		Classification:    "TOOL_ALLOWED",
		Reason:            strings.Repeat("x", 800),
		DecisionSource:    "test",
		Status:            types.ToolAuditStatusAllowed,
	})
	if err != nil {
		t.Fatalf("insert audit: %v", err)
	}

	var reason string
	var allowed bool
	err = pool.QueryRow(ctx, `
SELECT reason, allowed
FROM mcp_gateway_tool_call_audits
WHERE tenant_id = 'tenant-mcp-test' AND audit_id = 'audit-1'`).Scan(&reason, &allowed)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !allowed || len([]rune(reason)) != 512 {
		t.Fatalf("unexpected audit row allowed=%v reason_len=%d", allowed, len([]rune(reason)))
	}
}

const migrationSQL = `
CREATE TABLE IF NOT EXISTS mcp_gateway_tool_call_audits (
    tenant_id TEXT NOT NULL,
    audit_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    device_id TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    skill_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    tool_action TEXT NOT NULL,
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id TEXT NOT NULL DEFAULT '',
    risk_level TEXT NOT NULL DEFAULT '',
    intent TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL DEFAULT '',
    input_sha256 TEXT NOT NULL DEFAULT '',
    allowed BOOLEAN NOT NULL DEFAULT FALSE,
    requires_approval BOOLEAN NOT NULL DEFAULT FALSE,
    permission_version BIGINT NOT NULL DEFAULT 0,
    classification TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    decision_source TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, audit_id),
    CONSTRAINT ck_mcp_gateway_tool_call_audits_status
        CHECK (status IN ('ALLOWED', 'BLOCKED', 'FAILED'))
);
`
