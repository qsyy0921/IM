CREATE TABLE IF NOT EXISTS message_compliance_external_proofs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    external_proof_ref TEXT NOT NULL,
    status TEXT NOT NULL,
    provider TEXT NOT NULL,
    proof_hash TEXT NOT NULL,
    verified_by TEXT NOT NULL,
    verified_at TIMESTAMPTZ NOT NULL,
    revoked_by TEXT NOT NULL DEFAULT '',
    revoked_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT uq_message_compliance_external_proofs_ref UNIQUE (tenant_id, external_proof_ref),
    CONSTRAINT ck_message_compliance_external_proofs_status CHECK (status IN ('VERIFIED', 'REVOKED')),
    CONSTRAINT ck_message_compliance_external_proofs_ref CHECK (external_proof_ref <> ''),
    CONSTRAINT ck_message_compliance_external_proofs_provider CHECK (provider <> ''),
    CONSTRAINT ck_message_compliance_external_proofs_hash CHECK (proof_hash <> ''),
    CONSTRAINT ck_message_compliance_external_proofs_revoked_fields CHECK (
        (status = 'VERIFIED' AND revoked_by = '' AND revoked_at IS NULL)
        OR
        (status = 'REVOKED' AND revoked_by <> '' AND revoked_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_message_compliance_external_proofs_status_updated
    ON message_compliance_external_proofs (tenant_id, status, updated_at DESC);
