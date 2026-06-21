package pgvector

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const DefaultTableName = "vector_embedding_items"

type Store struct {
	pool  *pgxpool.Pool
	table string
}

type Item struct {
	TenantID           string
	VectorItemID       string
	BackendVectorID    string
	CollectionID       string
	EmbeddingModelRef  string
	EmbeddingValues    []float32
	Dimension          int
	SourceRefHash      string
	ChunkHash          string
	VisibilityScope    string
	VisibilityVersion  int64
	PolicyVersion      string
	DataClass          string
	TombstoneStatus    string
	DeleteProofID      string
	RetentionPolicyRef string
	CorrelationID      string
	CausationID        string
	TraceID            string
	UpdatedAt          time.Time
}

type SearchQuery struct {
	TenantID        string
	QueryEmbedding  []float32
	CollectionID    string
	VisibilityScope string
	PolicyVersion   string
	TopK            int
	MinScore        float64
}

type SearchResult struct {
	VectorItemID    string
	BackendVectorID string
	CollectionID    string
	Score           float64
}

func NewStore(pool *pgxpool.Pool, table string) (*Store, error) {
	table = strings.TrimSpace(table)
	if table == "" {
		table = DefaultTableName
	}
	quoted, err := quoteIdentifier(table)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool, table: quoted}, nil
}

func (store *Store) EnsureSchema(ctx context.Context, dimension int) error {
	if err := store.ready(); err != nil {
		return err
	}
	if dimension <= 0 {
		return errors.New("pgvector dimension must be positive")
	}
	_, err := store.pool.Exec(ctx, fmt.Sprintf(`
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS %s (
    tenant_id TEXT NOT NULL,
    vector_item_id TEXT NOT NULL,
    backend_vector_id TEXT NOT NULL,
    collection_id TEXT NOT NULL,
    embedding_model_ref TEXT NOT NULL,
    dimension INTEGER NOT NULL,
    embedding vector(%d) NOT NULL,
    source_ref_hash TEXT NOT NULL,
    chunk_hash TEXT NOT NULL,
    visibility_scope TEXT NOT NULL,
    visibility_version BIGINT NOT NULL,
    policy_version TEXT NOT NULL,
    data_class TEXT NOT NULL,
    tombstone_status TEXT NOT NULL DEFAULT 'NONE',
    delete_proof_id TEXT NOT NULL DEFAULT '',
    retention_policy_ref TEXT NOT NULL DEFAULT '',
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, vector_item_id),
    UNIQUE (tenant_id, backend_vector_id),
    CONSTRAINT ck_vector_embedding_items_dimension CHECK (dimension > 0),
    CONSTRAINT ck_vector_embedding_items_status CHECK (status IN ('ACTIVE', 'DELETED')),
    CONSTRAINT ck_vector_embedding_items_tombstone CHECK (tombstone_status IN ('NONE', 'TOMBSTONED'))
);

CREATE INDEX IF NOT EXISTS idx_vector_embedding_items_filter
    ON %s (tenant_id, collection_id, status, tombstone_status, visibility_scope, policy_version, updated_at DESC);
`, store.table, dimension, store.table))
	return err
}

func (store *Store) Upsert(ctx context.Context, item Item) error {
	if err := store.ready(); err != nil {
		return err
	}
	item = normalizeItem(item)
	if err := validateItem(item); err != nil {
		return err
	}
	vectorLiteral, err := VectorLiteral(item.EmbeddingValues)
	if err != nil {
		return err
	}
	_, err = store.pool.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s (
    tenant_id, vector_item_id, backend_vector_id, collection_id, embedding_model_ref,
    dimension, embedding, source_ref_hash, chunk_hash, visibility_scope, visibility_version,
    policy_version, data_class, tombstone_status, delete_proof_id, retention_policy_ref,
    correlation_id, causation_id, trace_id, status, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7::vector, $8, $9, $10, $11,
    $12, $13, $14, $15, $16,
    $17, $18, $19, 'ACTIVE', $20
) ON CONFLICT (tenant_id, vector_item_id) DO UPDATE SET
    backend_vector_id = EXCLUDED.backend_vector_id,
    collection_id = EXCLUDED.collection_id,
    embedding_model_ref = EXCLUDED.embedding_model_ref,
    dimension = EXCLUDED.dimension,
    embedding = EXCLUDED.embedding,
    source_ref_hash = EXCLUDED.source_ref_hash,
    chunk_hash = EXCLUDED.chunk_hash,
    visibility_scope = EXCLUDED.visibility_scope,
    visibility_version = EXCLUDED.visibility_version,
    policy_version = EXCLUDED.policy_version,
    data_class = EXCLUDED.data_class,
    tombstone_status = EXCLUDED.tombstone_status,
    delete_proof_id = EXCLUDED.delete_proof_id,
    retention_policy_ref = EXCLUDED.retention_policy_ref,
    correlation_id = EXCLUDED.correlation_id,
    causation_id = EXCLUDED.causation_id,
    trace_id = EXCLUDED.trace_id,
    status = 'ACTIVE',
    updated_at = EXCLUDED.updated_at
`, store.table),
		item.TenantID, item.VectorItemID, item.BackendVectorID, item.CollectionID, item.EmbeddingModelRef,
		item.Dimension, vectorLiteral, item.SourceRefHash, item.ChunkHash, item.VisibilityScope, item.VisibilityVersion,
		item.PolicyVersion, item.DataClass, item.TombstoneStatus, item.DeleteProofID, item.RetentionPolicyRef,
		item.CorrelationID, item.CausationID, item.TraceID, item.UpdatedAt,
	)
	return err
}

func (store *Store) Delete(ctx context.Context, tenantID, vectorItemID, deleteProofID string, deletedAt time.Time) error {
	if err := store.ready(); err != nil {
		return err
	}
	tenantID = strings.TrimSpace(tenantID)
	vectorItemID = strings.TrimSpace(vectorItemID)
	deleteProofID = strings.TrimSpace(deleteProofID)
	if tenantID == "" || vectorItemID == "" || deleteProofID == "" {
		return errors.New("tenant_id, vector_item_id, and delete_proof_id are required")
	}
	if deletedAt.IsZero() {
		deletedAt = time.Now().UTC()
	}
	_, err := store.pool.Exec(ctx, fmt.Sprintf(`
UPDATE %s
SET status = 'DELETED',
    tombstone_status = 'TOMBSTONED',
    delete_proof_id = $3,
    updated_at = $4
WHERE tenant_id = $1
  AND vector_item_id = $2
`, store.table), tenantID, vectorItemID, deleteProofID, deletedAt)
	return err
}

func (store *Store) Search(ctx context.Context, query SearchQuery) ([]SearchResult, error) {
	if err := store.ready(); err != nil {
		return nil, err
	}
	query = normalizeQuery(query)
	if err := validateQuery(query); err != nil {
		return nil, err
	}
	vectorLiteral, err := VectorLiteral(query.QueryEmbedding)
	if err != nil {
		return nil, err
	}
	rows, err := store.pool.Query(ctx, fmt.Sprintf(`
SELECT vector_item_id,
       backend_vector_id,
       collection_id,
       1 - (embedding <=> $2::vector) AS score
FROM %s
WHERE tenant_id = $1
  AND status = 'ACTIVE'
  AND tombstone_status = 'NONE'
  AND ($3 = '' OR collection_id = $3)
  AND ($4 = '' OR visibility_scope = $4)
  AND ($5 = '' OR policy_version = $5)
  AND 1 - (embedding <=> $2::vector) >= $6
ORDER BY embedding <=> $2::vector
LIMIT $7
`, store.table), query.TenantID, vectorLiteral, query.CollectionID, query.VisibilityScope, query.PolicyVersion, query.MinScore, query.TopK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []SearchResult{}
	for rows.Next() {
		var result SearchResult
		if err := rows.Scan(&result.VectorItemID, &result.BackendVectorID, &result.CollectionID, &result.Score); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

func VectorLiteral(values []float32) (string, error) {
	if len(values) == 0 {
		return "", errors.New("embedding values are required")
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		floatValue := float64(value)
		if math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
			return "", errors.New("embedding values must be finite")
		}
		parts = append(parts, strconv.FormatFloat(floatValue, 'g', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]", nil
}

func (store *Store) ready() error {
	if store == nil || store.pool == nil {
		return errors.New("pgvector store is not configured")
	}
	if store.table == "" {
		return errors.New("pgvector table is not configured")
	}
	return nil
}

func normalizeItem(item Item) Item {
	item.TenantID = strings.TrimSpace(item.TenantID)
	item.VectorItemID = strings.TrimSpace(item.VectorItemID)
	item.BackendVectorID = strings.TrimSpace(item.BackendVectorID)
	item.CollectionID = strings.TrimSpace(item.CollectionID)
	item.EmbeddingModelRef = strings.TrimSpace(item.EmbeddingModelRef)
	item.SourceRefHash = strings.TrimSpace(item.SourceRefHash)
	item.ChunkHash = strings.TrimSpace(item.ChunkHash)
	item.VisibilityScope = strings.TrimSpace(item.VisibilityScope)
	item.PolicyVersion = strings.TrimSpace(item.PolicyVersion)
	item.DataClass = strings.ToUpper(strings.TrimSpace(item.DataClass))
	item.TombstoneStatus = strings.ToUpper(strings.TrimSpace(item.TombstoneStatus))
	if item.TombstoneStatus == "" {
		item.TombstoneStatus = "NONE"
	}
	item.DeleteProofID = strings.TrimSpace(item.DeleteProofID)
	item.RetentionPolicyRef = strings.TrimSpace(item.RetentionPolicyRef)
	item.CorrelationID = strings.TrimSpace(item.CorrelationID)
	item.CausationID = strings.TrimSpace(item.CausationID)
	item.TraceID = strings.TrimSpace(item.TraceID)
	if item.Dimension == 0 {
		item.Dimension = len(item.EmbeddingValues)
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = time.Now().UTC()
	}
	return item
}

func validateItem(item Item) error {
	if item.TenantID == "" || item.VectorItemID == "" || item.BackendVectorID == "" {
		return errors.New("tenant_id, vector_item_id, and backend_vector_id are required")
	}
	if item.CollectionID == "" || item.EmbeddingModelRef == "" {
		return errors.New("collection_id and embedding_model_ref are required")
	}
	if item.Dimension <= 0 || item.Dimension != len(item.EmbeddingValues) {
		return errors.New("embedding dimension must match embedding values")
	}
	if item.SourceRefHash == "" || item.ChunkHash == "" {
		return errors.New("source_ref_hash and chunk_hash are required")
	}
	if item.VisibilityScope == "" || item.VisibilityVersion <= 0 || item.PolicyVersion == "" {
		return errors.New("visibility and policy metadata are required")
	}
	if item.DataClass == "" {
		return errors.New("data_class is required")
	}
	if item.TombstoneStatus != "NONE" && item.TombstoneStatus != "TOMBSTONED" {
		return errors.New("tombstone_status is unsupported")
	}
	return nil
}

func normalizeQuery(query SearchQuery) SearchQuery {
	query.TenantID = strings.TrimSpace(query.TenantID)
	query.CollectionID = strings.TrimSpace(query.CollectionID)
	query.VisibilityScope = strings.TrimSpace(query.VisibilityScope)
	query.PolicyVersion = strings.TrimSpace(query.PolicyVersion)
	if query.TopK <= 0 {
		query.TopK = 10
	}
	if query.MinScore < 0 {
		query.MinScore = 0
	}
	return query
}

func validateQuery(query SearchQuery) error {
	if query.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	if query.TopK <= 0 {
		return errors.New("top_k must be positive")
	}
	if len(query.QueryEmbedding) == 0 {
		return errors.New("query embedding is required")
	}
	return nil
}

func quoteIdentifier(value string) (string, error) {
	if value == "" {
		return "", errors.New("identifier is required")
	}
	parts := strings.Split(value, ".")
	for _, part := range parts {
		if !isSafeIdentifier(part) {
			return "", fmt.Errorf("unsafe pgvector identifier %q", value)
		}
	}
	for index, part := range parts {
		parts[index] = `"` + part + `"`
	}
	return strings.Join(parts, "."), nil
}

func isSafeIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, r := range value {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || index > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}
