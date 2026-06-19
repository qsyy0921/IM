CREATE TABLE IF NOT EXISTS agent_proposals (
    tenant_id TEXT NOT NULL,
    proposal_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    objective TEXT NOT NULL DEFAULT '',
    skill_id TEXT NOT NULL,
    prepared_audit_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    tool_action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL DEFAULT '',
    risk_level TEXT NOT NULL DEFAULT '',
    intent TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    proposal_text TEXT NOT NULL DEFAULT '',
    requires_approval BOOLEAN NOT NULL DEFAULT TRUE,
    allowed BOOLEAN NOT NULL DEFAULT FALSE,
    permission_version BIGINT NOT NULL DEFAULT 0,
    classification TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    decision_source TEXT NOT NULL DEFAULT '',
    evidence_pack_id TEXT NOT NULL DEFAULT '',
    citations_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    agent_version TEXT NOT NULL DEFAULT '',
    generated_by_llm BOOLEAN NOT NULL DEFAULT FALSE,
    approval_id TEXT NOT NULL DEFAULT '',
    approved_by_user_id TEXT NOT NULL DEFAULT '',
    approved_at TIMESTAMPTZ NULL,
    approval_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, proposal_id),
    CONSTRAINT ck_agent_proposals_tool_action
        CHECK (tool_action IN ('CALL', 'APPROVE', 'EXECUTE')),
    CONSTRAINT ck_agent_proposals_status
        CHECK (status IN ('PROPOSED', 'BLOCKED', 'INSUFFICIENT_EVIDENCE', 'APPROVED')),
    CONSTRAINT ck_agent_proposals_approved_fields
        CHECK (
            (status = 'APPROVED' AND approval_id <> '' AND approved_by_user_id <> '' AND approved_at IS NOT NULL)
            OR
            (status <> 'APPROVED' AND approval_id = '' AND approved_by_user_id = '' AND approved_at IS NULL)
        )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_proposals_approval
    ON agent_proposals (tenant_id, approval_id)
    WHERE approval_id <> '';

CREATE INDEX IF NOT EXISTS idx_agent_proposals_tenant_user_created
    ON agent_proposals (tenant_id, user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_agent_proposals_tenant_status_created
    ON agent_proposals (tenant_id, status, created_at DESC);
