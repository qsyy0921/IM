package grpc

import (
	"errors"
	"time"

	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

const serviceOverloadedRetryDelay = 500 * time.Millisecond

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
	publicMessage := publicErrorMessage(err)
	st := status.New(grpcCode, publicMessage)
	messageError := &messagev1.MessageError{
		Code:          messageErrorCode,
		Message:       publicMessage,
		Retryable:     retryable,
		CorrelationId: correlationID,
	}
	if errors.Is(err, types.ErrServiceOverloaded) {
		retryDelay := serviceOverloadedRetryDelay
		if dynamicDelay, ok := types.ServiceOverloadedRetryDelay(err); ok {
			retryDelay = dynamicDelay
		}
		withDetails, detailErr := st.WithDetails(messageError, &errdetails.RetryInfo{
			RetryDelay: durationpb.New(retryDelay),
		})
		if detailErr != nil {
			return st.Err()
		}
		return withDetails.Err()
	}

	withDetails, detailErr := st.WithDetails(messageError)
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
	case errors.Is(err, types.ErrConversationNotFound):
		return codes.NotFound, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_CONVERSATION_NOT_FOUND, false
	case errors.Is(err, types.ErrMessageNotFound):
		return codes.NotFound, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSPECIFIED, false
	case errors.Is(err, types.ErrInvalidMessageState):
		return codes.FailedPrecondition, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSPECIFIED, false
	case errors.Is(err, types.ErrIdempotencyConflict):
		return codes.Aborted, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_IDEMPOTENCY_CONFLICT, false
	case errors.Is(err, types.ErrSequencerUnavailable):
		return codes.Unavailable, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_SEQUENCER_UNAVAILABLE, true
	case errors.Is(err, types.ErrDBWriteFailed):
		return codes.Unavailable, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_DB_WRITE_FAILED, true
	case errors.Is(err, types.ErrOutboxWriteFailed):
		return codes.Unavailable, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_OUTBOX_WRITE_FAILED, true
	case errors.Is(err, types.ErrServiceOverloaded):
		return codes.Unavailable, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_SERVICE_OVERLOADED, true
	case errors.Is(err, types.ErrDependencyVersion):
		return codes.Unavailable, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSPECIFIED, true
	case errors.Is(err, types.ErrDependencyUnavailable):
		return codes.Unavailable, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSPECIFIED, true
	default:
		return codes.Internal, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSPECIFIED, false
	}
}

func publicErrorMessage(err error) string {
	var invalid invalidArgumentError
	switch {
	case errors.As(err, &invalid):
		return invalid.Error()
	case errors.Is(err, types.ErrPermissionDenied):
		return "permission denied"
	case errors.Is(err, types.ErrUnsupportedMessageType):
		return "unsupported message type"
	case errors.Is(err, types.ErrConversationNotFound):
		return "conversation not found"
	case errors.Is(err, types.ErrMessageNotFound):
		return "message not found"
	case errors.Is(err, types.ErrInvalidMessageState):
		return "invalid message state"
	case errors.Is(err, types.ErrIdempotencyConflict):
		return "idempotency conflict"
	case errors.Is(err, types.ErrSequencerUnavailable):
		return "sequencer unavailable"
	case errors.Is(err, types.ErrDBWriteFailed):
		return "database write failed"
	case errors.Is(err, types.ErrOutboxWriteFailed):
		return "outbox write failed"
	case errors.Is(err, types.ErrServiceOverloaded):
		return "service overloaded"
	case errors.Is(err, types.ErrDependencyVersion):
		return "dependency version mismatch"
	case errors.Is(err, types.ErrDependencyUnavailable):
		return "dependency unavailable"
	default:
		return "message service internal error"
	}
}
