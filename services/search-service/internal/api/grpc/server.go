package grpc

import (
	"context"
	"errors"

	searchv1 "github.com/qsyy0921/IM/api/proto/nexusim/search/v1"
	"github.com/qsyy0921/IM/services/search-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type SearchMessagesExecutor interface {
	Execute(context.Context, types.SearchMessagesCommand) (types.SearchMessagesResult, error)
}

type Server struct {
	searchv1.UnimplementedSearchServiceServer
	searchMessages SearchMessagesExecutor
}

func NewServer(searchMessages SearchMessagesExecutor) *Server {
	return &Server{searchMessages: searchMessages}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	searchv1.RegisterSearchServiceServer(registrar, server)
}

func (server *Server) SearchMessages(
	ctx context.Context,
	request *searchv1.SearchMessagesRequest,
) (*searchv1.SearchMessagesResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.searchMessages.Execute(ctx, types.SearchMessagesCommand{
		AuthContext:    auth,
		Query:          request.GetQuery(),
		ConversationID: types.ConversationID(request.GetConversationId()),
		AfterSeq:       request.GetAfterSeq(),
		Limit:          int(request.GetLimit()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	items := make([]*searchv1.SearchMessageHit, 0, len(result.Items))
	for _, item := range result.Items {
		ranges := make([]*searchv1.HighlightRange, 0, len(item.HighlightRanges))
		for _, highlight := range item.HighlightRanges {
			ranges = append(ranges, &searchv1.HighlightRange{
				Start: highlight.Start,
				End:   highlight.End,
			})
		}
		items = append(items, &searchv1.SearchMessageHit{
			ConversationId:    string(item.ConversationID),
			MessageId:         item.MessageID,
			ConversationSeq:   item.ConversationSeq,
			SourceEventId:     item.SourceEventID,
			SenderId:          string(item.SenderID),
			MessageType:       item.MessageType,
			Snippet:           item.Snippet,
			HighlightRanges:   ranges,
			OccurredAtUnixMs:  item.OccurredAt.UnixMilli(),
			VisibilityVersion: item.VisibilityVersion,
		})
	}
	return &searchv1.SearchMessagesResponse{
		Items:             items,
		NextCursor:        result.NextCursor,
		ProjectionVersion: result.ProjectionVersion,
	}, nil
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid argument")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrSearchUnavailable):
		return status.Error(codes.Unavailable, "search unavailable")
	case errors.Is(err, types.ErrProjectionStale):
		return status.Error(codes.Unavailable, "search projection stale")
	case errors.Is(err, types.ErrDBReadFailed):
		return status.Error(codes.Unavailable, "search read failed")
	case errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "search write failed")
	case errors.Is(err, types.ErrServiceOverloaded):
		return status.Error(codes.Unavailable, "service overloaded")
	default:
		return status.Error(codes.Internal, "search service internal error")
	}
}

func authFromProto(ctx context.Context, auth *searchv1.AuthContext) (types.AuthContext, bool) {
	if verified, ok := verifiedAuthFromContext(ctx); ok {
		if auth != nil {
			if verified.TraceID == "" {
				verified.TraceID = auth.GetTraceId()
			}
			if verified.RequestID == "" {
				verified.RequestID = auth.GetRequestId()
			}
		}
		return verified, true
	}
	if auth == nil {
		return types.AuthContext{}, false
	}
	return types.AuthContext{
		TenantID:  types.TenantID(auth.GetTenantId()),
		UserID:    types.UserID(auth.GetUserId()),
		DeviceID:  auth.GetDeviceId(),
		SessionID: auth.GetSessionId(),
		TraceID:   auth.GetTraceId(),
		RequestID: auth.GetRequestId(),
	}, true
}
