CREATE TABLE IF NOT EXISTS vector_backend_items (
    tenant_id TEXT NOT NULL,
    backend_type TEXT NOT NULL,
    backend_vector_id TEXT NOT NULL,
    vector_item_id TEXT NOT NULL,
    collection_id TEXT NOT NULL,
    source_ref_hash TEXT NOT NULL,
    embedding_vector_hash TEXT NOT NULL,
    dimension INTEGER NOT NULL,
    status TEXT NOT NULL,
    tombstone_status TEXT NOT NULL DEFAULT 'NONE',
    indexed_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, backend_type, backend_vector_id),
    CONSTRAINT fk_vector_backend_items_item FOREIGN KEY (tenant_id, vector_item_id)
        REFERENCES vector_items (tenant_id, vector_item_id) ON DELETE CASCADE,
    CONSTRAINT ck_vector_backend_items_backend_type CHECK (backend_type IN (
        'POSTGRES_TEST',
        'PGVECTOR',
        'MILVUS',
        'OPENSEARCH_VECTOR'
    )),
    CONSTRAINT ck_vector_backend_items_dimension CHECK (dimension > 0),
    CONSTRAINT ck_vector_backend_items_status CHECK (status IN ('ACTIVE', 'DELETED')),
    CONSTRAINT ck_vector_backend_items_tombstone CHECK (tombstone_status IN ('NONE', 'TOMBSTONED'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_vector_backend_items_item
    ON vector_backend_items (tenant_id, backend_type, vector_item_id);

CREATE INDEX IF NOT EXISTS idx_vector_backend_items_search
    ON vector_backend_items (tenant_id, backend_type, collection_id, status, tombstone_status, updated_at DESC);

UPDATE vector_items
SET backend_vector_id = vector_item_id
WHERE backend_vector_id = '';

INSERT INTO vector_backend_items (
    tenant_id, backend_type, backend_vector_id, vector_item_id, collection_id,
    source_ref_hash, embedding_vector_hash, dimension, status, tombstone_status,
    indexed_at, deleted_at, updated_at
)
SELECT
    vi.tenant_id,
    vc.backend_type,
    vi.backend_vector_id,
    vi.vector_item_id,
    vi.collection_id,
    vi.source_ref_hash,
    vi.embedding_vector_hash,
    vi.dimension,
    CASE WHEN vi.status = 'TOMBSTONED' THEN 'DELETED' ELSE 'ACTIVE' END,
    vi.tombstone_status,
    vi.created_at,
    CASE WHEN vi.status = 'TOMBSTONED' THEN vi.updated_at ELSE NULL END,
    vi.updated_at
FROM vector_items vi
JOIN vector_collections vc
  ON vc.tenant_id = vi.tenant_id AND vc.collection_id = vi.collection_id
ON CONFLICT (tenant_id, backend_type, backend_vector_id) DO NOTHING;
