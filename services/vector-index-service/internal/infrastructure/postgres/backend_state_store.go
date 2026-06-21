package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

type BackendStateStore struct {
	pool *pgxpool.Pool
}

func NewBackendStateStore(pool *pgxpool.Pool) *BackendStateStore {
	return &BackendStateStore{pool: pool}
}

func (store *BackendStateStore) ConfirmActiveBackendItem(ctx context.Context, item types.VectorItem) error {
	if store == nil || store.pool == nil {
		return types.NewDBReadFailed("vector backend state store is not configured")
	}
	var status string
	var tombstoneStatus string
	var dimension int
	var embeddingVectorHash string
	err := store.pool.QueryRow(ctx, `
SELECT status, tombstone_status, dimension, embedding_vector_hash
FROM vector_backend_items
WHERE tenant_id = $1
  AND backend_type = $2
  AND backend_vector_id = $3
  AND vector_item_id = $4
`, string(item.TenantID), types.BackendTypePostgresTest, item.BackendVectorID, item.VectorItemID).
		Scan(&status, &tombstoneStatus, &dimension, &embeddingVectorHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.NewFailedPrecondition("postgres-test backend item is missing")
		}
		return types.NewDBReadFailed(err.Error())
	}
	if status != types.BackendItemStatusActive || tombstoneStatus != types.TombstoneStatusNone {
		return types.NewFailedPrecondition("postgres-test backend item is not active")
	}
	if dimension != item.Dimension {
		return types.NewFailedPrecondition("postgres-test backend item dimension mismatch")
	}
	if embeddingVectorHash != item.EmbeddingVectorHash {
		return types.NewFailedPrecondition("postgres-test backend item embedding hash mismatch")
	}
	return nil
}
