CREATE TABLE IF NOT EXISTS policy_decision_audit_outbox (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    actor_user_key TEXT NOT NULL,
    device_key TEXT NOT NULL DEFAULT '',
    conversation_key TEXT NOT NULL,
    message_key TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    message_id_present BOOLEAN NOT NULL DEFAULT false,
    direct_peer_context_present BOOLEAN NOT NULL DEFAULT false,
    direct_peer_key TEXT NOT NULL DEFAULT '',
    allowed BOOLEAN NOT NULL,
    permission_version BIGINT NOT NULL,
    classification TEXT NOT NULL,
    reason_code TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL DEFAULT 'policy.message_action_decision.v1',
    event_version TEXT NOT NULL DEFAULT 'v1',
    producer TEXT NOT NULL DEFAULT 'policy-service',
    partition_key TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    payload_json JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    retry_count INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    dead_lettered_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (action IN ('SEND', 'EDIT', 'REVOKE', 'DELETE')),
    CHECK (permission_version > 0),
    CHECK (classification <> ''),
    CHECK (actor_user_key <> ''),
    CHECK (conversation_key <> ''),
    CHECK (event_type <> ''),
    CHECK (event_version <> ''),
    CHECK (producer <> ''),
    CHECK (partition_key <> ''),
    CHECK (status IN ('PENDING', 'PUBLISHED', 'DLQ')),
    CHECK (retry_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_policy_decision_audit_outbox_ready
    ON policy_decision_audit_outbox (status, available_at, id)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_policy_decision_audit_outbox_tenant_created
    ON policy_decision_audit_outbox (tenant_id, created_at DESC);
