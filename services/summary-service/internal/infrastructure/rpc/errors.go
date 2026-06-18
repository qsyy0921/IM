package rpc

import (
	"context"
	"errors"

	"github.com/qsyy0921/IM/services/summary-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func mapRetrievalError(err error) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
	case codes.PermissionDenied:
		return types.ErrPermissionDenied
	case codes.InvalidArgument:
		return types.ErrInvalidArgument
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.ErrRetrievalUnavailable
	default:
		if errors.Is(err, context.Canceled) {
			return err
		}
		return types.ErrRetrievalUnavailable
	}
}
