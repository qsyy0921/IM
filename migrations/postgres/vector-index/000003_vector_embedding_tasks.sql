CREATE TABLE IF NOT EXISTS vector_embedding_tasks (
    tenant_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    source_service TEXT NOT NULL,
    collection_type TEXT NOT NULL,
    source_ref_hash TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_version BIGINT NOT NULL,
    source_hash TEXT NOT NULL,
    chunk_hash TEXT NOT NULL,
    input_preview_redacted TEXT NOT NULL,
    input_hash TEXT NOT NULL,
    input_schema_version INTEGER NOT NULL DEFAULT 1,
    embedding_model_ref TEXT NOT NULL,
    dimension INTEGER NOT NULL,
    visibility_scope TEXT NOT NULL,
    visibility_version BIGINT NOT NULL,
    policy_version TEXT NOT NULL,
    data_class TEXT NOT NULL,
    delete_proof_id TEXT NOT NULL DEFAULT '',
    retention_policy_ref TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'PENDING',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    claimed_at TIMESTAMPTZ,
    claim_deadline TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, task_id),
    CONSTRAINT ck_vector_embedding_tasks_collection_type CHECK (collection_type IN (
        'KNOWLEDGE_CHUNK',
        'MEMORY_EVENT',
        'SEARCH_DOCUMENT',
        'PROFILE_AGGREGATE',
        'EVAL_FIXTURE'
    )),
    CONSTRAINT ck_vector_embedding_tasks_status CHECK (status IN ('PENDING', 'RUNNING', 'COMPLETED', 'FAILED')),
    CONSTRAINT ck_vector_embedding_tasks_version CHECK (source_version > 0),
    CONSTRAINT ck_vector_embedding_tasks_input_schema CHECK (input_schema_version > 0),
    CONSTRAINT ck_vector_embedding_tasks_dimension CHECK (dimension > 0),
    CONSTRAINT ck_vector_embedding_tasks_visibility_version CHECK (visibility_version > 0),
    CONSTRAINT ck_vector_embedding_tasks_attempt_count CHECK (attempt_count >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_vector_embedding_tasks_idempotency
    ON vector_embedding_tasks (tenant_id, source_service, source_id, source_version, embedding_model_ref, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_vector_embedding_tasks_ready
    ON vector_embedding_tasks (tenant_id, status, available_at, claim_deadline, created_at)
    WHERE status IN ('PENDING', 'RUNNING');
