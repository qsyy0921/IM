CREATE TABLE IF NOT EXISTS knowledge_sources (
    tenant_id TEXT NOT NULL,
    source_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL DEFAULT '',
    command_hash TEXT NOT NULL,
    source_type TEXT NOT NULL,
    source_ref TEXT NOT NULL,
    source_ref_hash TEXT NOT NULL,
    media_object_ref TEXT NOT NULL DEFAULT '',
    owner_ref TEXT NOT NULL,
    visibility_scope TEXT NOT NULL,
    data_class TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    mime_type TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    source_version TEXT NOT NULL,
    retention_policy_ref TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, source_id),
    CONSTRAINT ck_knowledge_sources_source_type
        CHECK (source_type IN ('MEDIA_OBJECT', 'WEB_PAGE', 'ADMIN_UPLOAD', 'CONNECTOR_RECORD', 'MANUAL_MARKDOWN')),
    CONSTRAINT ck_knowledge_sources_data_class
        CHECK (data_class IN ('LOW_SENSITIVE', 'BUSINESS_INTERNAL', 'USER_CONTENT', 'SECURITY_SENSITIVE')),
    CONSTRAINT ck_knowledge_sources_status
        CHECK (status IN ('ACTIVE', 'TOMBSTONED')),
    CONSTRAINT ck_knowledge_sources_size
        CHECK (size_bytes >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_knowledge_sources_idempotency
    ON knowledge_sources (tenant_id, idempotency_key)
    WHERE idempotency_key <> '';

CREATE INDEX IF NOT EXISTS idx_knowledge_sources_updated
    ON knowledge_sources (tenant_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS knowledge_documents (
    tenant_id TEXT NOT NULL,
    document_id TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_version TEXT NOT NULL,
    parser_profile TEXT NOT NULL,
    mime_type TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    page_count INTEGER NOT NULL DEFAULT 0,
    language TEXT NOT NULL DEFAULT '',
    document_hash TEXT NOT NULL,
    parse_status TEXT NOT NULL,
    parser_failure_class TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, document_id),
    CONSTRAINT ck_knowledge_documents_parse_status
        CHECK (parse_status IN ('PARSED', 'FAILED')),
    CONSTRAINT ck_knowledge_documents_counts
        CHECK (size_bytes >= 0 AND page_count >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_knowledge_documents_source_version
    ON knowledge_documents (tenant_id, source_id, source_version, document_hash);

CREATE TABLE IF NOT EXISTS knowledge_chunks (
    tenant_id TEXT NOT NULL,
    chunk_id TEXT NOT NULL,
    document_id TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_version TEXT NOT NULL,
    chunk_index INTEGER NOT NULL,
    chunk_hash TEXT NOT NULL,
    chunk_text_encrypted_ref TEXT NOT NULL DEFAULT '',
    chunk_preview_redacted TEXT NOT NULL DEFAULT '',
    visibility_scope TEXT NOT NULL,
    data_class TEXT NOT NULL,
    policy_version TEXT NOT NULL,
    chunk_version TEXT NOT NULL,
    status TEXT NOT NULL,
    tombstone_status TEXT NOT NULL DEFAULT 'ACTIVE',
    delete_proof_id TEXT NOT NULL DEFAULT '',
    embedding_status TEXT NOT NULL DEFAULT 'PENDING',
    vector_status TEXT NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, chunk_id),
    CONSTRAINT ck_knowledge_chunks_index
        CHECK (chunk_index >= 0),
    CONSTRAINT ck_knowledge_chunks_data_class
        CHECK (data_class IN ('LOW_SENSITIVE', 'BUSINESS_INTERNAL', 'USER_CONTENT', 'SECURITY_SENSITIVE')),
    CONSTRAINT ck_knowledge_chunks_status
        CHECK (status IN ('READY', 'TOMBSTONED')),
    CONSTRAINT ck_knowledge_chunks_tombstone
        CHECK (tombstone_status IN ('ACTIVE', 'TOMBSTONED')),
    CONSTRAINT ck_knowledge_chunks_embedding
        CHECK (embedding_status IN ('PENDING', 'REQUESTED', 'DONE', 'FAILED')),
    CONSTRAINT ck_knowledge_chunks_vector
        CHECK (vector_status IN ('PENDING', 'HANDOFF', 'INDEXED', 'FAILED'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_knowledge_chunks_manifest
    ON knowledge_chunks (tenant_id, document_id, chunk_index);

CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_source
    ON knowledge_chunks (tenant_id, source_id, document_id, chunk_index);

CREATE TABLE IF NOT EXISTS knowledge_ingestion_jobs (
    tenant_id TEXT NOT NULL,
    job_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL DEFAULT '',
    command_hash TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_version TEXT NOT NULL,
    job_type TEXT NOT NULL,
    parser_profile TEXT NOT NULL,
    chunk_profile TEXT NOT NULL,
    embedding_policy_ref TEXT NOT NULL DEFAULT '',
    vector_policy_ref TEXT NOT NULL DEFAULT '',
    requested_by TEXT NOT NULL,
    status TEXT NOT NULL,
    retry_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ NULL,
    failure_class TEXT NOT NULL DEFAULT '',
    public_error TEXT NOT NULL DEFAULT '',
    document_id TEXT NOT NULL DEFAULT '',
    chunk_count INTEGER NOT NULL DEFAULT 0,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ NULL,
    PRIMARY KEY (tenant_id, job_id),
    CONSTRAINT ck_knowledge_jobs_type
        CHECK (job_type IN ('INGEST', 'REINGEST', 'REBUILD_CHUNKS', 'REFRESH_METADATA', 'TOMBSTONE', 'DELETE_PROOF_REPAIR')),
    CONSTRAINT ck_knowledge_jobs_status
        CHECK (status IN ('PENDING', 'PARSING', 'CHUNKING', 'EMBEDDING_REQUESTED', 'INDEX_HANDOFF', 'DONE', 'FAILED', 'RETRYING', 'CANCELED', 'TOMBSTONED')),
    CONSTRAINT ck_knowledge_jobs_counts
        CHECK (retry_count >= 0 AND chunk_count >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_knowledge_jobs_idempotency
    ON knowledge_ingestion_jobs (tenant_id, source_id, idempotency_key)
    WHERE idempotency_key <> '';

CREATE TABLE IF NOT EXISTS knowledge_delete_proofs (
    tenant_id TEXT NOT NULL,
    delete_proof_id TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_version TEXT NOT NULL,
    proof_type TEXT NOT NULL,
    proof_ref_hash TEXT NOT NULL,
    requested_by TEXT NOT NULL,
    reason_class TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, delete_proof_id)
);

CREATE TABLE IF NOT EXISTS knowledge_outbox (
    event_id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    event_version INTEGER NOT NULL DEFAULT 1,
    partition_key TEXT NOT NULL,
    payload_json JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    retry_count INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at TIMESTAMPTZ NULL,
    published_at TIMESTAMPTZ NULL,
    dead_lettered_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_knowledge_outbox_status
        CHECK (status IN ('PENDING', 'PUBLISHED', 'DLQ'))
);

CREATE INDEX IF NOT EXISTS idx_knowledge_outbox_ready
    ON knowledge_outbox (status, available_at, next_retry_at, partition_key);
