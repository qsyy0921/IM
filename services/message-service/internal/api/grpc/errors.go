package grpc

import (
	"errors"

	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type invalidArgumentError struct {
	message string
}

func newInvalidArgument(message string) error {
	return invalidArgumentError{message: message}
}

func (e invalidArgumentError) Error() string {
	return e.message
}

func grpcError(err error, correlationID string) error {
	grpcCode, messageErrorCode, retryable := classifyError(err)
	st := status.New(grpcCode, err.Error())
	withDetails, detailErr := st.WithDetails(&messagev1.MessageError{
		Code:          messageErrorCode,
		Message:       err.Error(),
		Retryable:     retryable,
		CorrelationId: correlationID,
	})
	if detailErr != nil {
		return st.Err()
	}
	return withDetails.Err()
}

func classifyError(err error) (codes.Code, messagev1.MessageErrorCode, bool) {
	var invalid invalidArgumentError
	switch {
	case errors.As(err, &invalid):
		return codes.InvalidArgument, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSPECIFIED, false
	case errors.Is(err, types.ErrPermissionDenied):
		return codes.PermissionDenied, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_PERMISSION_DENIED, false
	case errors.Is(err, types.ErrUnsupportedMessageType):
		return codes.InvalidArgument, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSUPPORTED_MESSAGE_TYPE, false
	case errors.Is(err, types.ErrIdempotencyConflict):
		return codes.Aborted, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_IDEMPOTENCY_CONFLICT, false
	case errors.Is(err, types.ErrSequencerUnavailable):
		return codes.Unavailable, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_SEQUENCER_UNAVAILABLE, true
	case errors.Is(err, types.ErrDBWriteFailed):
		return codes.Unavailable, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_DB_WRITE_FAILED, true
	case errors.Is(err, types.ErrOutboxWriteFailed):
		return codes.Unavailable, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_OUTBOX_WRITE_FAILED, true
	case errors.Is(err, types.ErrDependencyVersion):
		return codes.Unavailable, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSPECIFIED, true
	default:
		return codes.Internal, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSPECIFIED, false
	}
}
