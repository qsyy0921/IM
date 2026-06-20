package app

import (
	"context"

	"github.com/qsyy0921/IM/services/model-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/model-gateway/internal/types"
)

type Repository interface {
	StartTextInvocation(ctx context.Context, prepared domain.PreparedTextGeneration) (types.ModelInvocation, bool, error)
	CompleteTextInvocation(ctx context.Context, invocation types.ModelInvocation) error
	GetModelInvocation(ctx context.Context, tenantID types.TenantID, invocationID string) (types.ModelInvocation, error)
}

type TextProvider interface {
	GenerateText(ctx context.Context, request domain.ProviderTextRequest) (domain.ProviderTextResult, error)
}

type InvocationIDGenerator interface {
	NewInvocationID() string
}
