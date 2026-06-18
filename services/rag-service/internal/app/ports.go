package app

import (
	"context"

	"github.com/qsyy0921/IM/services/rag-service/internal/types"
)

type RetrievalPort interface {
	RetrieveEvidence(context.Context, types.RetrieveEvidenceQuery) (types.RetrieveEvidenceResult, error)
}
