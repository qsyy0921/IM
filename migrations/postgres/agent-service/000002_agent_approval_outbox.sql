CREATE TABLE IF NOT EXISTS agent_approval_outbox (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    proposal_id TEXT NOT NULL,
    approval_id TEXT NOT NULL,
    prepared_audit_id TEXT NOT NULL,
    skill_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL DEFAULT '',
    risk_level TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL,
    event_version TEXT NOT NULL,
    mapping_version INT NOT NULL,
    partition_key TEXT NOT NULL,
    producer TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    payload_json JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    retry_count INT NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    dead_lettered_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_agent_approval_outbox_event_type
        CHECK (event_type IN ('agent.proposal.approved.v1')),
    CONSTRAINT ck_agent_approval_outbox_event_version
        CHECK (event_version <> ''),
    CONSTRAINT ck_agent_approval_outbox_mapping_version
        CHECK (mapping_version > 0),
    CONSTRAINT ck_agent_approval_outbox_retry_count
        CHECK (retry_count >= 0),
    CONSTRAINT ck_agent_approval_outbox_payload_json
        CHECK (jsonb_typeof(payload_json) = 'object'),
    CONSTRAINT ck_agent_approval_outbox_status
        CHECK (status IN ('PENDING', 'PUBLISHED', 'DLQ'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_approval_outbox_approval
    ON agent_approval_outbox (tenant_id, approval_id);

CREATE INDEX IF NOT EXISTS idx_agent_approval_outbox_ready
    ON agent_approval_outbox (status, COALESCE(next_retry_at, available_at), id)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_agent_approval_outbox_partition_blockers
    ON agent_approval_outbox (tenant_id, partition_key, status, id)
    WHERE status IN ('PENDING', 'DLQ');

CREATE INDEX IF NOT EXISTS idx_agent_approval_outbox_proposal
    ON agent_approval_outbox (tenant_id, proposal_id, created_at DESC);
