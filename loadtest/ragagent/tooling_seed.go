package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func seedBusinessToolingRows(ctx context.Context, cfg config) error {
	connectCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	pool, err := pgxpool.New(connectCtx, cfg.pgDSN)
	if err != nil {
		return fmt.Errorf("connect postgres for business tooling seed: %w", err)
	}
	defer pool.Close()

	now := time.Now().UTC().Truncate(time.Millisecond)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin business tooling seed: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
INSERT INTO skill_registry_definitions (
	tenant_id, skill_id, display_name, description, version, status, tool_name,
	allowed_actions_json, input_schema_json, output_schema_json, permission_scope,
	risk_level, requires_approval, audit_event_type, owner_service, tags_json,
	metadata_json, created_at, updated_at
) VALUES (
	$1, $2, $2, 'RAG agent business mutation skill for conversation note creation.',
	'v1', 'ACTIVE', $3, '[1,3]'::jsonb,
	'{"type":"object","additionalProperties":true}'::jsonb,
	'{"type":"object","additionalProperties":true}'::jsonb,
	$4, 'LOW', true, 'conversation.note.proposed.v1', 'agent-service',
	'["rag-agent-demo","business-mutation"]'::jsonb,
	'{"source":"loadtest/ragagent","purpose":"business_mutation_execute"}'::jsonb,
	$5, $5
)
ON CONFLICT (tenant_id, skill_id) DO UPDATE SET
	display_name = EXCLUDED.display_name,
	description = EXCLUDED.description,
	version = EXCLUDED.version,
	status = EXCLUDED.status,
	tool_name = EXCLUDED.tool_name,
	allowed_actions_json = EXCLUDED.allowed_actions_json,
	input_schema_json = EXCLUDED.input_schema_json,
	output_schema_json = EXCLUDED.output_schema_json,
	permission_scope = EXCLUDED.permission_scope,
	risk_level = EXCLUDED.risk_level,
	requires_approval = EXCLUDED.requires_approval,
	audit_event_type = EXCLUDED.audit_event_type,
	owner_service = EXCLUDED.owner_service,
	tags_json = EXCLUDED.tags_json,
	metadata_json = EXCLUDED.metadata_json,
	updated_at = EXCLUDED.updated_at
`, cfg.tenantID, defaultAgentSkillID, defaultAgentToolName, defaultAgentResourceType, now); err != nil {
		return fmt.Errorf("seed business skill registry definition: %w", err)
	}

	for _, action := range []string{"CALL", "EXECUTE"} {
		if _, err := tx.Exec(ctx, `
INSERT INTO policy_tool_action_rules (
	tenant_id, tool_name, action, resource_type, risk_level, allowed,
	requires_approval, permission_version, classification, reason, priority,
	enabled, source, updated_at
) VALUES (
	$1, $2, $3, $4, 'LOW', true, true, 42,
	'TOOL_APPROVAL_REQUIRED', 'rag-agent business mutation requires approval before execution',
	10, true, 'ragagent-demo', $5
)
ON CONFLICT (tenant_id, tool_name, action, resource_type, risk_level) DO UPDATE SET
	allowed = EXCLUDED.allowed,
	requires_approval = EXCLUDED.requires_approval,
	permission_version = EXCLUDED.permission_version,
	classification = EXCLUDED.classification,
	reason = EXCLUDED.reason,
	priority = EXCLUDED.priority,
	enabled = EXCLUDED.enabled,
	source = EXCLUDED.source,
	updated_at = EXCLUDED.updated_at
`, cfg.tenantID, defaultAgentToolName, action, defaultAgentResourceType, now); err != nil {
			return fmt.Errorf("seed business policy tool action rule %s: %w", action, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit business tooling seed: %w", err)
	}
	return nil
}
