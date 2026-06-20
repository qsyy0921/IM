CREATE TABLE IF NOT EXISTS vector_collections (
    tenant_id TEXT NOT NULL,
    collection_id TEXT NOT NULL,
    collection_type TEXT NOT NULL,
    backend_type TEXT NOT NULL DEFAULT 'POSTGRES_TEST',
    dimension INTEGER NOT NULL,
    embedding_model_ref TEXT NOT NULL,
    route_policy_ref TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    metadata_schema_version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, collection_id),
    CONSTRAINT ck_vector_collections_type CHECK (collection_type IN (
        'KNOWLEDGE_CHUNK',
        'MEMORY_EVENT',
        'SEARCH_DOCUMENT',
        'PROFILE_AGGREGATE',
        'EVAL_FIXTURE'
    )),
    CONSTRAINT ck_vector_collections_status CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT ck_vector_collections_dimension CHECK (dimension > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_vector_collections_model
    ON vector_collections (tenant_id, collection_type, embedding_model_ref, dimension);

CREATE TABLE IF NOT EXISTS vector_items (
    tenant_id TEXT NOT NULL,
    vector_item_id TEXT NOT NULL,
    collection_id TEXT NOT NULL,
    source_service TEXT NOT NULL,
    source_ref_hash TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_version BIGINT NOT NULL,
    source_hash TEXT NOT NULL,
    chunk_hash TEXT NOT NULL,
    embedding_model_ref TEXT NOT NULL,
    embedding_vector_hash TEXT NOT NULL,
    backend_vector_id TEXT NOT NULL DEFAULT '',
    dimension INTEGER NOT NULL,
    visibility_scope TEXT NOT NULL,
    visibility_version BIGINT NOT NULL,
    policy_version TEXT NOT NULL,
    data_class TEXT NOT NULL,
    tombstone_status TEXT NOT NULL DEFAULT 'NONE',
    delete_proof_id TEXT NOT NULL DEFAULT '',
    retention_policy_ref TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    command_hash TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, vector_item_id),
    CONSTRAINT fk_vector_items_collection FOREIGN KEY (tenant_id, collection_id)
        REFERENCES vector_collections (tenant_id, collection_id),
    CONSTRAINT ck_vector_items_version CHECK (source_version > 0),
    CONSTRAINT ck_vector_items_dimension CHECK (dimension > 0),
    CONSTRAINT ck_vector_items_visibility_version CHECK (visibility_version > 0),
    CONSTRAINT ck_vector_items_tombstone_status CHECK (tombstone_status IN ('NONE', 'TOMBSTONED')),
    CONSTRAINT ck_vector_items_status CHECK (status IN ('INDEXED', 'TOMBSTONED'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_vector_items_idempotency
    ON vector_items (tenant_id, source_service, source_id, source_version, embedding_model_ref, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_vector_items_search
    ON vector_items (tenant_id, collection_id, status, tombstone_status, updated_at DESC);

CREATE TABLE IF NOT EXISTS vector_index_jobs (
    tenant_id TEXT NOT NULL,
    job_id TEXT NOT NULL,
    collection_id TEXT NOT NULL,
    vector_item_id TEXT NOT NULL,
    job_type TEXT NOT NULL,
    status TEXT NOT NULL,
    retry_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ,
    failure_class TEXT NOT NULL DEFAULT '',
    public_error TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    command_hash TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, job_id),
    CONSTRAINT fk_vector_jobs_collection FOREIGN KEY (tenant_id, collection_id)
        REFERENCES vector_collections (tenant_id, collection_id),
    CONSTRAINT ck_vector_jobs_type CHECK (job_type IN ('UPSERT', 'TOMBSTONE', 'REBUILD')),
    CONSTRAINT ck_vector_jobs_status CHECK (status IN (
        'PENDING',
        'EMBEDDING_REQUESTED',
        'VECTOR_UPSERTING',
        'INDEXED',
        'TOMBSTONED',
        'FAILED',
        'RETRYING',
        'CANCELED'
    )),
    CONSTRAINT ck_vector_jobs_retry_count CHECK (retry_count >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_vector_jobs_idempotency
    ON vector_index_jobs (tenant_id, vector_item_id, job_type, idempotency_key);

CREATE TABLE IF NOT EXISTS vector_tombstones (
    tenant_id TEXT NOT NULL,
    tombstone_id TEXT NOT NULL,
    vector_item_id TEXT NOT NULL,
    source_ref_hash TEXT NOT NULL,
    delete_proof_id TEXT NOT NULL,
    reason_class TEXT NOT NULL,
    backend_delete_status TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    command_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, tombstone_id),
    CONSTRAINT ck_vector_tombstones_backend_status CHECK (backend_delete_status IN ('PENDING', 'DELETED'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_vector_tombstones_idempotency
    ON vector_tombstones (tenant_id, vector_item_id, idempotency_key);

CREATE TABLE IF NOT EXISTS vector_rebuild_checkpoints (
    tenant_id TEXT NOT NULL,
    rebuild_job_id TEXT NOT NULL,
    collection_id TEXT NOT NULL,
    source_service TEXT NOT NULL,
    partition_key TEXT NOT NULL,
    cursor_value TEXT NOT NULL,
    status TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, rebuild_job_id, partition_key),
    CONSTRAINT ck_vector_rebuild_checkpoints_status CHECK (status IN ('PENDING', 'RUNNING', 'COMPLETED', 'FAILED'))
);

CREATE TABLE IF NOT EXISTS vector_outbox (
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
    next_retry_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    dead_lettered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_vector_outbox_status CHECK (status IN ('PENDING', 'PUBLISHED', 'DLQ'))
);

CREATE INDEX IF NOT EXISTS idx_vector_jobs_status
    ON vector_index_jobs (tenant_id, status, created_at);

CREATE INDEX IF NOT EXISTS idx_vector_outbox_ready
    ON vector_outbox (status, available_at, created_at)
    WHERE status = 'PENDING';
