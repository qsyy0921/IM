package grpc

import (
	"context"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

type verifiedAuthKey struct{}

func ContextWithVerifiedAuth(ctx context.Context, auth types.AuthContext) context.Context {
	return context.WithValue(ctx, verifiedAuthKey{}, auth.Normalized())
}

func verifiedAuthFromContext(ctx context.Context) (types.AuthContext, bool) {
	auth, ok := ctx.Value(verifiedAuthKey{}).(types.AuthContext)
	if !ok {
		return types.AuthContext{}, false
	}
	return auth.Normalized(), true
}
