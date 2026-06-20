CREATE TABLE IF NOT EXISTS audit_records (
    tenant_id TEXT NOT NULL,
    audit_id TEXT NOT NULL,
    audit_stream TEXT NOT NULL,
    source_service TEXT NOT NULL,
    source_event_id TEXT NOT NULL,
    record_type TEXT NOT NULL,
    actor_ref TEXT NOT NULL DEFAULT '',
    subject_ref TEXT NOT NULL DEFAULT '',
    resource_ref TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    outcome TEXT NOT NULL,
    reason_code TEXT NOT NULL DEFAULT '',
    risk_level TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    attributes_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    canonical_json_hash TEXT NOT NULL,
    previous_record_hash TEXT NOT NULL DEFAULT '',
    record_hash TEXT NOT NULL,
    idempotency_key TEXT NOT NULL DEFAULT '',
    command_hash TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (tenant_id, audit_id),
    UNIQUE (tenant_id, source_service, source_event_id),
    UNIQUE (tenant_id, idempotency_key),
    UNIQUE (tenant_id, audit_stream, record_hash),
    CONSTRAINT ck_audit_records_attributes_object CHECK (jsonb_typeof(attributes_json) = 'object'),
    CONSTRAINT ck_audit_records_required_fields CHECK (
        btrim(tenant_id) <> ''
        AND btrim(audit_id) <> ''
        AND btrim(audit_stream) <> ''
        AND btrim(source_service) <> ''
        AND btrim(source_event_id) <> ''
        AND btrim(record_type) <> ''
        AND btrim(action) <> ''
        AND btrim(outcome) <> ''
        AND btrim(canonical_json_hash) <> ''
        AND btrim(record_hash) <> ''
        AND btrim(command_hash) <> ''
    )
);

CREATE TABLE IF NOT EXISTS audit_hash_segments (
    tenant_id TEXT NOT NULL,
    segment_id TEXT NOT NULL,
    audit_stream TEXT NOT NULL,
    sequence BIGINT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    record_count BIGINT NOT NULL DEFAULT 0,
    first_record_hash TEXT NOT NULL DEFAULT '',
    last_record_hash TEXT NOT NULL DEFAULT '',
    segment_root_hash TEXT NOT NULL DEFAULT '',
    previous_segment_hash TEXT NOT NULL DEFAULT '',
    seal_status TEXT NOT NULL DEFAULT 'OPEN',
    sealed_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, segment_id),
    UNIQUE (tenant_id, audit_stream, sequence),
    CONSTRAINT ck_audit_hash_segments_status CHECK (seal_status IN ('OPEN', 'SEALED')),
    CONSTRAINT ck_audit_hash_segments_counts CHECK (record_count >= 0)
);

CREATE TABLE IF NOT EXISTS audit_outbox (
    event_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    event_version INT NOT NULL DEFAULT 1,
    partition_key TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'PENDING',
    retry_count INT NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    dead_lettered_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, event_id),
    CONSTRAINT ck_audit_outbox_status CHECK (status IN ('PENDING', 'PUBLISHED', 'DLQ')),
    CONSTRAINT ck_audit_outbox_retry_count CHECK (retry_count >= 0),
    CONSTRAINT ck_audit_outbox_payload_object CHECK (jsonb_typeof(payload_json) = 'object'),
    CONSTRAINT ck_audit_outbox_required_fields CHECK (
        btrim(event_id) <> ''
        AND btrim(tenant_id) <> ''
        AND btrim(aggregate_type) <> ''
        AND btrim(aggregate_id) <> ''
        AND btrim(event_type) <> ''
        AND event_version > 0
        AND btrim(partition_key) <> ''
    )
);

CREATE INDEX IF NOT EXISTS idx_audit_records_stream_time
    ON audit_records (tenant_id, audit_stream, occurred_at DESC, audit_id DESC);

CREATE INDEX IF NOT EXISTS idx_audit_records_source
    ON audit_records (tenant_id, source_service, source_event_id);

CREATE INDEX IF NOT EXISTS idx_audit_outbox_ready
    ON audit_outbox (tenant_id, status, available_at, created_at)
    WHERE status = 'PENDING';
