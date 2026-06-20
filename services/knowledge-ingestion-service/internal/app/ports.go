package app

import (
	"context"

	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/domain"
	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/types"
)

type Repository interface {
	CreateKnowledgeSource(ctx context.Context, prepared domain.PreparedKnowledgeSource) (types.KnowledgeSource, bool, error)
	SubmitIngestionJob(ctx context.Context, prepared domain.PreparedIngestionJob) (types.KnowledgeIngestionJob, bool, error)
	GetIngestionJob(ctx context.Context, tenantID types.TenantID, jobID string) (types.KnowledgeIngestionJob, error)
	ListKnowledgeChunks(ctx context.Context, command types.ListKnowledgeChunksCommand) ([]types.KnowledgeChunk, string, error)
}

type IDGenerator interface {
	NewSourceID() string
	NewJobID() string
	NewDocumentID() string
	NewChunkID(index int) string
}
