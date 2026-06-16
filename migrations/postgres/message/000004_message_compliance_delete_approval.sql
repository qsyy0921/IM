CREATE TABLE IF NOT EXISTS message_compliance_delete_approvals (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    approval_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'APPROVED',
    external_proof_ref TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    approved_by TEXT NOT NULL,
    approved_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    consumed_by TEXT NOT NULL DEFAULT '',
    consumed_event_id TEXT NOT NULL DEFAULT '',
    consumed_at TIMESTAMPTZ,
    canceled_by TEXT NOT NULL DEFAULT '',
    canceled_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_message_compliance_delete_approvals_tenant_approval UNIQUE (tenant_id, approval_id),
    CONSTRAINT ck_message_compliance_delete_approvals_status CHECK (status IN ('APPROVED', 'CONSUMED', 'CANCELED')),
    CONSTRAINT ck_message_compliance_delete_approvals_external_proof_ref CHECK (external_proof_ref <> ''),
    CONSTRAINT ck_message_compliance_delete_approvals_consumed_fields CHECK (
        (status <> 'CONSUMED' AND consumed_at IS NULL)
        OR (status = 'CONSUMED' AND consumed_at IS NOT NULL AND consumed_event_id <> '')
    ),
    CONSTRAINT ck_message_compliance_delete_approvals_canceled_fields CHECK (
        (status <> 'CANCELED' AND canceled_at IS NULL)
        OR (status = 'CANCELED' AND canceled_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_message_compliance_delete_approvals_approved_message
    ON message_compliance_delete_approvals (tenant_id, conversation_id, message_id)
    WHERE status = 'APPROVED';

CREATE INDEX IF NOT EXISTS idx_message_compliance_delete_approvals_status_updated
    ON message_compliance_delete_approvals (tenant_id, status, updated_at DESC);
