package rpc

import (
	"context"
	"errors"

	"github.com/qsyy0921/IM/services/rag-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func mapRetrievalError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return types.ErrRetrievalUnavailable
	}
	st, ok := status.FromError(err)
	if !ok {
		return types.ErrRetrievalUnavailable
	}
	switch st.Code() {
	case codes.InvalidArgument:
		return types.ErrInvalidArgument
	case codes.PermissionDenied:
		return types.ErrPermissionDenied
	case codes.Unavailable, codes.DeadlineExceeded, codes.Unimplemented:
		return types.ErrRetrievalUnavailable
	default:
		return types.ErrRetrievalUnavailable
	}
}
