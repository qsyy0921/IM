package app

import (
	"context"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/domain"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

type VectorRepository interface {
	UpsertVectorItem(context.Context, domain.PreparedUpsert) (types.VectorItem, types.VectorIndexJob, bool, error)
	TombstoneVectorItem(context.Context, domain.PreparedTombstone) (types.VectorItem, types.VectorIndexJob, string, bool, error)
	SearchVectors(context.Context, types.SearchVectorsCommand) ([]types.VectorSearchResult, error)
	RequestVectorRebuild(context.Context, domain.PreparedRebuild) (types.VectorIndexJob, types.VectorRebuildCheckpoint, bool, error)
	GetVectorIndexJob(context.Context, types.GetVectorIndexJobCommand) (types.VectorIndexJob, error)
}

type IDGenerator interface {
	NewVectorItemID() string
	NewJobID() string
	NewTombstoneID() string
}
