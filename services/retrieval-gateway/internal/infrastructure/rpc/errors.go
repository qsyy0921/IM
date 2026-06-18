package rpc

import (
	"context"
	"errors"

	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func mapSearchError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return types.ErrSearchUnavailable
	}
	st, ok := status.FromError(err)
	if !ok {
		return types.ErrSearchUnavailable
	}
	switch st.Code() {
	case codes.InvalidArgument:
		return types.ErrInvalidArgument
	case codes.PermissionDenied:
		return types.ErrPermissionDenied
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.ErrSearchUnavailable
	default:
		return types.ErrSearchUnavailable
	}
}

func mapMemoryError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return types.ErrMemoryUnavailable
	}
	st, ok := status.FromError(err)
	if !ok {
		return types.ErrMemoryUnavailable
	}
	switch st.Code() {
	case codes.InvalidArgument:
		return types.ErrInvalidArgument
	case codes.PermissionDenied:
		return types.ErrPermissionDenied
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.ErrMemoryUnavailable
	default:
		return types.ErrMemoryUnavailable
	}
}

func mapPolicyError(err error) error {
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
