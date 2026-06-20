package grpc

import (
	"context"

	"github.com/qsyy0921/IM/services/control-plane-service/internal/types"
)

type verifiedAuthContextKey struct{}

func ContextWithVerifiedAuth(ctx context.Context, auth types.AuthContext) context.Context {
	return context.WithValue(ctx, verifiedAuthContextKey{}, auth)
}

func verifiedAuthFromContext(ctx context.Context) (types.AuthContext, bool) {
	auth, ok := ctx.Value(verifiedAuthContextKey{}).(types.AuthContext)
	return auth, ok
}
