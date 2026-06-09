package grpc

import (
	"context"
	"errors"

	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PullInboxExecutor interface {
	Execute(context.Context, types.PullInboxCommand) (types.PullInboxResult, error)
}

type AckDeliveryExecutor interface {
	Execute(context.Context, types.AckDeliveryCommand) (types.AckDeliveryResult, error)
}

type Server struct {
	deliveryv1.UnimplementedDeliveryServiceServer
	pullInbox   PullInboxExecutor
	ackDelivery AckDeliveryExecutor
}

func NewServer(pullInbox PullInboxExecutor, ackDelivery AckDeliveryExecutor) *Server {
	return &Server{pullInbox: pullInbox, ackDelivery: ackDelivery}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	deliveryv1.RegisterDeliveryServiceServer(registrar, server)
}

func (s *Server) PullInbox(
	ctx context.Context,
	request *deliveryv1.PullInboxRequest,
) (*deliveryv1.PullInboxResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth := request.GetAuthContext()
	result, err := s.pullInbox.Execute(ctx, types.PullInboxCommand{
		AuthContext: types.AuthContext{
			TenantID:  types.TenantID(auth.GetTenantId()),
			UserID:    types.UserID(auth.GetUserId()),
			DeviceID:  auth.GetDeviceId(),
			SessionID: auth.GetSessionId(),
			TraceID:   auth.GetTraceId(),
			RequestID: auth.GetRequestId(),
		},
		ConversationID: types.ConversationID(request.GetConversationId()),
		AfterSeq:       request.GetAfterSeq(),
		Limit:          int(request.GetLimit()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	items := make([]*deliveryv1.InboxItem, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, &deliveryv1.InboxItem{
			ConversationId:  string(item.ConversationID),
			ConversationSeq: item.ConversationSeq,
			EventId:         item.EventID,
			EventType:       item.EventType,
			MessageId:       item.MessageID,
			SenderId:        string(item.SenderID),
			PayloadJson:     []byte(item.PayloadJSON),
			CreatedAtUnixMs: item.CreatedAt.UnixMilli(),
		})
	}
	return &deliveryv1.PullInboxResponse{
		Items:   items,
		NextSeq: result.NextSeq,
		HasMore: result.HasMore,
	}, nil
}

func (s *Server) AckDelivery(
	ctx context.Context,
	request *deliveryv1.AckDeliveryRequest,
) (*deliveryv1.AckDeliveryResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth := request.GetAuthContext()
	result, err := s.ackDelivery.Execute(ctx, types.AckDeliveryCommand{
		AuthContext: types.AuthContext{
			TenantID:  types.TenantID(auth.GetTenantId()),
			UserID:    types.UserID(auth.GetUserId()),
			DeviceID:  auth.GetDeviceId(),
			SessionID: auth.GetSessionId(),
			TraceID:   auth.GetTraceId(),
			RequestID: auth.GetRequestId(),
		},
		ConversationID: types.ConversationID(request.GetConversationId()),
		ReceivedSeq:    request.GetReceivedSeq(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &deliveryv1.AckDeliveryResponse{
		TenantId:        string(result.TenantID),
		UserId:          string(result.UserID),
		DeviceId:        result.DeviceID,
		ConversationId:  string(result.ConversationID),
		LastReceivedSeq: result.LastReceivedSeq,
	}, nil
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid argument")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrCursorRegression):
		return status.Error(codes.FailedPrecondition, "cursor regression")
	case errors.Is(err, types.ErrAckOutOfVisibleRange):
		return status.Error(codes.FailedPrecondition, "ack out of visible range")
	case errors.Is(err, types.ErrDBReadFailed):
		return status.Error(codes.Unavailable, "delivery read failed")
	case errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "delivery write failed")
	case errors.Is(err, types.ErrServiceOverloaded):
		return status.Error(codes.Unavailable, "service overloaded")
	default:
		return status.Error(codes.Internal, "delivery service internal error")
	}
}
