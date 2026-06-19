package rpc

import (
	"errors"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func mapSkillError(err error) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
	case codes.NotFound:
		return types.ErrSkillNotFound
	case codes.InvalidArgument:
		return types.ErrInvalidArgument
	case codes.PermissionDenied:
		return types.ErrPermissionDenied
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.ErrSkillCatalogUnavailable
	default:
		return errors.Join(types.ErrSkillCatalogUnavailable, err)
	}
}

func mapPolicyError(err error) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
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

func mapAgentProposalError(err error) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
	case codes.InvalidArgument:
		return types.ErrInvalidArgument
	case codes.PermissionDenied:
		return types.ErrPermissionDenied
	case codes.NotFound:
		return types.ErrProposalNotApproved
	case codes.FailedPrecondition:
		if parsed, ok := status.FromError(err); ok && parsed.Message() == "proposal not approved" {
			return types.ErrProposalNotApproved
		}
		return types.ErrProposalMismatch
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.ErrProposalApprovalUnavailable
	default:
		return errors.Join(types.ErrProposalApprovalUnavailable, err)
	}
}
