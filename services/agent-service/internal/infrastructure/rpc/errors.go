package rpc

import (
	"errors"

	"github.com/qsyy0921/IM/services/agent-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func mapRetrievalError(err error) error {
	if err == nil {
		return nil
	}
	code := status.Code(err)
	switch code {
	case codes.InvalidArgument:
		return types.ErrInvalidArgument
	case codes.PermissionDenied:
		return types.ErrPermissionDenied
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.ErrRetrievalUnavailable
	default:
		return errors.Join(types.ErrRetrievalUnavailable, err)
	}
}

func mapPolicyError(err error) error {
	if err == nil {
		return nil
	}
	code := status.Code(err)
	switch code {
	case codes.InvalidArgument:
		return types.ErrInvalidArgument
	case codes.PermissionDenied:
		return types.ErrPermissionDenied
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.ErrToolPolicyUnavailable
	default:
		return errors.Join(types.ErrToolPolicyUnavailable, err)
	}
}

func mapMCPGatewayError(err error) error {
	if err == nil {
		return nil
	}
	code := status.Code(err)
	switch code {
	case codes.InvalidArgument:
		return types.ErrInvalidArgument
	case codes.PermissionDenied:
		return types.ErrPermissionDenied
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.ErrToolPrepareUnavailable
	default:
		return errors.Join(types.ErrToolPrepareUnavailable, err)
	}
}
