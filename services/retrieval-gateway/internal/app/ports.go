package app

import (
	"context"

	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
)

type SearchPort interface {
	SearchMessages(context.Context, types.SearchQuery) (types.SearchResult, error)
}

type MemoryPort interface {
	QueryMemoryEvents(context.Context, types.MemoryQuery) (types.MemoryResult, error)
	GetMemoryEvent(context.Context, types.MemoryEventLookup) (types.MemoryEventLookupResult, error)
	ListProfileAggregates(context.Context, types.ProfileAggregateQuery) (types.ProfileAggregateResult, error)
}

type VectorPort interface {
	SearchVectors(context.Context, types.VectorQuery) (types.VectorResult, error)
}

type PolicyPort interface {
	CheckRetrieveEvidence(context.Context, types.RetrievalPolicyCheck) (types.RetrievalPolicyDecision, error)
}
