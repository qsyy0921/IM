package app

import (
	"context"

	"github.com/qsyy0921/IM/services/summary-service/internal/types"
)

type RetrievalPort interface {
	RetrieveEvidence(context.Context, types.RetrieveEvidenceQuery) (types.RetrieveEvidenceResult, error)
}

type SummaryProvider interface {
	GenerateSummary(context.Context, types.SummaryGenerationRequest) (types.SummaryGenerationResult, error)
}
