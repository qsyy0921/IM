package grpc

import (
	"context"
	"errors"

	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MarkReadExecutor interface {
	Execute(context.Context, types.MarkReadCommand) (types.MarkReadResult, error)
}

type GetReceiptStateExecutor interface {
	Execute(context.Context, types.GetReceiptStateCommand) (types.GetReceiptStateResult, error)
}

type Server struct {
	receiptv1.UnimplementedReceiptServiceServer
	markRead        MarkReadExecutor
	getReceiptState GetReceiptStateExecutor
}

func NewServer(markRead MarkReadExecutor, getReceiptState GetReceiptStateExecutor) *Server {
	return &Server{markRead: markRead, getReceiptState: getReceiptState}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	receiptv1.RegisterReceiptServiceServer(registrar, server)
}

func (server *Server) MarkRead(
	ctx context.Context,
	request *receiptv1.MarkReadRequest,
) (*receiptv1.MarkReadResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth := request.GetAuthContext()
	result, err := server.markRead.Execute(ctx, types.MarkReadCommand{
		AuthContext: types.AuthContext{
			TenantID:  types.TenantID(auth.GetTenantId()),
			UserID:    types.UserID(auth.GetUserId()),
			DeviceID:  auth.GetDeviceId(),
			SessionID: auth.GetSessionId(),
			TraceID:   auth.GetTraceId(),
			RequestID: auth.GetRequestId(),
		},
		ConversationID: types.ConversationID(request.GetConversationId()),
		ReadSeq:        request.GetReadSeq(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &receiptv1.MarkReadResponse{
		TenantId:       string(result.TenantID),
		UserId:         string(result.UserID),
		ConversationId: string(result.ConversationID),
		LastReadSeq:    result.LastReadSeq,
	}, nil
}

func (server *Server) GetReceiptState(
	ctx context.Context,
	request *receiptv1.GetReceiptStateRequest,
) (*receiptv1.GetReceiptStateResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth := request.GetAuthContext()
	result, err := server.getReceiptState.Execute(ctx, types.GetReceiptStateCommand{
		AuthContext: types.AuthContext{
			TenantID:  types.TenantID(auth.GetTenantId()),
			UserID:    types.UserID(auth.GetUserId()),
			DeviceID:  auth.GetDeviceId(),
			SessionID: auth.GetSessionId(),
			TraceID:   auth.GetTraceId(),
			RequestID: auth.GetRequestId(),
		},
		ConversationID:  types.ConversationID(request.GetConversationId()),
		MessageID:       request.GetMessageId(),
		ConversationSeq: request.GetConversationSeq(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	receivers := make([]*receiptv1.ReceiptUserState, 0, len(result.Receivers))
	for _, receiver := range result.Receivers {
		receivers = append(receivers, &receiptv1.ReceiptUserState{
			UserId:           string(receiver.UserID),
			ReceivedSeq:      receiver.ReceivedSeq,
			ReceivedAtUnixMs: receiver.ReceivedAt.UnixMilli(),
			ReadSeq:          receiver.ReadSeq,
			ReadAtUnixMs:     receiver.ReadAt.UnixMilli(),
		})
	}
	return &receiptv1.GetReceiptStateResponse{
		ConversationId:    string(result.ConversationID),
		ConversationSeq:   result.ConversationSeq,
		MessageId:         result.MessageID,
		ReceivedUserCount: int32(result.ReceivedUserCount),
		ReadUserCount:     int32(result.ReadUserCount),
		VisibilityMode:    toProtoVisibility(result.VisibilityMode),
		Receivers:         receivers,
	}, nil
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid argument")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrReadOutOfVisibleRange):
		return status.Error(codes.FailedPrecondition, "read out of visible range")
	case errors.Is(err, types.ErrReadOutOfReceivedRange):
		return status.Error(codes.FailedPrecondition, "read out of received range")
	case errors.Is(err, types.ErrReceiptNotFound):
		return status.Error(codes.NotFound, "receipt not found")
	case errors.Is(err, types.ErrProjectionLagging):
		return status.Error(codes.Unavailable, "receipt projection lagging")
	case errors.Is(err, types.ErrDBReadFailed):
		return status.Error(codes.Unavailable, "receipt read failed")
	case errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "receipt write failed")
	case errors.Is(err, types.ErrServiceOverloaded):
		return status.Error(codes.Unavailable, "service overloaded")
	default:
		return status.Error(codes.Internal, "receipt service internal error")
	}
}

func toProtoVisibility(mode string) receiptv1.ReceiptVisibilityMode {
	switch mode {
	case types.ReceiptVisibilityDetailed:
		return receiptv1.ReceiptVisibilityMode_RECEIPT_VISIBILITY_MODE_DETAILED
	case types.ReceiptVisibilityCountOnly:
		return receiptv1.ReceiptVisibilityMode_RECEIPT_VISIBILITY_MODE_COUNT_ONLY
	case types.ReceiptVisibilityHidden:
		return receiptv1.ReceiptVisibilityMode_RECEIPT_VISIBILITY_MODE_HIDDEN
	default:
		return receiptv1.ReceiptVisibilityMode_RECEIPT_VISIBILITY_MODE_UNSPECIFIED
	}
}
