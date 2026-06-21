package postgres

import (
	"context"
	"strings"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

type TenantRebuildStore struct {
	repository *Repository
	tenantID   types.TenantID
}

func NewTenantRebuildStore(repository *Repository, tenantID string) TenantRebuildStore {
	return TenantRebuildStore{
		repository: repository,
		tenantID:   types.TenantID(strings.TrimSpace(tenantID)),
	}
}

func (store TenantRebuildStore) ClaimRebuildTasks(ctx context.Context, limit int) ([]types.VectorRebuildTask, error) {
	if store.tenantID == "" {
		return store.repository.ClaimRebuildTasks(ctx, limit)
	}
	return store.repository.ClaimRebuildTasksForTenant(ctx, store.tenantID, limit)
}

func (store TenantRebuildStore) ContinueRebuildTask(ctx context.Context, task types.VectorRebuildTask, cursorValue string) error {
	return store.repository.ContinueRebuildTask(ctx, task, cursorValue)
}

func (store TenantRebuildStore) CompleteRebuildTask(ctx context.Context, task types.VectorRebuildTask) error {
	return store.repository.CompleteRebuildTask(ctx, task)
}
