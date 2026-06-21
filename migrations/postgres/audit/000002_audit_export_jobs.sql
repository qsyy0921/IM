CREATE TABLE IF NOT EXISTS audit_export_jobs (
    tenant_id TEXT NOT NULL,
    export_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    audit_stream TEXT NOT NULL DEFAULT '',
    record_type TEXT NOT NULL DEFAULT '',
    source_service TEXT NOT NULL DEFAULT '',
    filter_hash TEXT NOT NULL,
    redaction_profile TEXT NOT NULL,
    requested_by_ref TEXT NOT NULL,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    manifest_ref TEXT NOT NULL DEFAULT '',
    record_count BIGINT NOT NULL DEFAULT 0,
    completed_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    public_error TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    command_hash TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, export_id),
    UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT ck_audit_export_jobs_status CHECK (status IN ('PENDING', 'RUNNING', 'COMPLETED', 'FAILED', 'CANCELED')),
    CONSTRAINT ck_audit_export_jobs_record_count CHECK (record_count >= 0),
    CONSTRAINT ck_audit_export_jobs_required_fields CHECK (
        btrim(tenant_id) <> ''
        AND btrim(export_id) <> ''
        AND btrim(filter_hash) <> ''
        AND btrim(redaction_profile) <> ''
        AND btrim(requested_by_ref) <> ''
        AND btrim(idempotency_key) <> ''
        AND btrim(command_hash) <> ''
    )
);

CREATE INDEX IF NOT EXISTS idx_audit_export_jobs_status
    ON audit_export_jobs (tenant_id, status, requested_at DESC);
