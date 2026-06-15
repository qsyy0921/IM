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

type ListReceiptStatesExecutor interface {
	Execute(context.Context, types.ListReceiptStatesCommand) (types.ListReceiptStatesResult, error)
}

type ListConversationsExecutor interface {
	Execute(context.Context, types.ListConversationsCommand) (types.ListConversationsResult, error)
}

type ArchiveConversationExecutor interface {
	Execute(context.Context, types.ArchiveConversationCommand) (types.ArchiveConversationResult, error)
}

type PinConversationExecutor interface {
	Execute(context.Context, types.PinConversationCommand) (types.PinConversationResult, error)
}

type MuteConversationExecutor interface {
	Execute(context.Context, types.MuteConversationCommand) (types.MuteConversationResult, error)
}

type Server struct {
	receiptv1.UnimplementedReceiptServiceServer
	markRead            MarkReadExecutor
	getReceiptState     GetReceiptStateExecutor
	listReceiptStates   ListReceiptStatesExecutor
	listConversations   ListConversationsExecutor
	archiveConversation ArchiveConversationExecutor
	pinConversation     PinConversationExecutor
	muteConversation    MuteConversationExecutor
}

func NewServer(
	markRead MarkReadExecutor,
	getReceiptState GetReceiptStateExecutor,
	listReceiptStates ListReceiptStatesExecutor,
	listConversations ListConversationsExecutor,
	archiveConversation ArchiveConversationExecutor,
	pinConversation PinConversationExecutor,
	muteConversation MuteConversationExecutor,
) *Server {
	return &Server{
		markRead:            markRead,
		getReceiptState:     getReceiptState,
		listReceiptStates:   listReceiptStates,
		listConversations:   listConversations,
		archiveConversation: archiveConversation,
		pinConversation:     pinConversation,
		muteConversation:    muteConversation,
	}
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
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.markRead.Execute(ctx, types.MarkReadCommand{
		AuthContext:    auth,
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
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.getReceiptState.Execute(ctx, types.GetReceiptStateCommand{
		AuthContext:     auth,
		ConversationID:  types.ConversationID(request.GetConversationId()),
		MessageID:       request.GetMessageId(),
		ConversationSeq: request.GetConversationSeq(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return receiptStateResponse(result), nil
}

func (server *Server) ListReceiptStates(
	ctx context.Context,
	request *receiptv1.ListReceiptStatesRequest,
) (*receiptv1.ListReceiptStatesResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	items := make([]types.ReceiptStateQuery, 0, len(request.GetItems()))
	for _, item := range request.GetItems() {
		items = append(items, types.ReceiptStateQuery{
			MessageID:       item.GetMessageId(),
			ConversationSeq: item.GetConversationSeq(),
		})
	}
	result, err := server.listReceiptStates.Execute(ctx, types.ListReceiptStatesCommand{
		AuthContext:    auth,
		ConversationID: types.ConversationID(request.GetConversationId()),
		Items:          items,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	response := &receiptv1.ListReceiptStatesResponse{
		Items: make([]*receiptv1.GetReceiptStateResponse, 0, len(result.Items)),
	}
	for _, item := range result.Items {
		response.Items = append(response.Items, receiptStateResponse(item))
	}
	return response, nil
}

func receiptStateResponse(result types.GetReceiptStateResult) *receiptv1.GetReceiptStateResponse {
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
	}
}

func (server *Server) ListConversations(
	ctx context.Context,
	request *receiptv1.ListConversationsRequest,
) (*receiptv1.ListConversationsResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.listConversations.Execute(ctx, types.ListConversationsCommand{
		AuthContext:     auth,
		Limit:           int(request.GetLimit()),
		PageCursor:      request.GetPageCursor(),
		Sort:            conversationListSortFromProto(request.GetSort()),
		IncludeArchived: request.GetIncludeArchived(),
		UnreadOnly:      request.GetUnreadOnly(),
		PinnedOnly:      request.GetPinnedOnly(),
		MutedOnly:       request.GetMutedOnly(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	items := make([]*receiptv1.ConversationSummary, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, &receiptv1.ConversationSummary{
			ConversationId:      string(item.ConversationID),
			LastVisibleSeq:      item.LastVisibleSeq,
			LastMessageId:       item.LastMessageID,
			LastSenderId:        string(item.LastSenderID),
			LastSourceEventType: item.LastSourceEventType,
			UnreadCount:         item.UnreadCount,
			LastReadSeq:         item.LastReadSeq,
			UpdatedAtUnixMs:     item.UpdatedAt.UnixMilli(),
			Archived:            item.Archived,
			Pinned:              item.Pinned,
			Muted:               item.Muted,
		})
	}
	return &receiptv1.ListConversationsResponse{
		Items:          items,
		NextPageCursor: result.NextPageCursor,
		ProjectionWatermark: &receiptv1.ProjectionWatermark{
			Source:          result.ProjectionWatermark.Source,
			OffsetValue:     result.ProjectionWatermark.OffsetValue,
			UpdatedAtUnixMs: result.ProjectionWatermark.UpdatedAt.UnixMilli(),
		},
	}, nil
}

func (server *Server) ArchiveConversation(
	ctx context.Context,
	request *receiptv1.ArchiveConversationRequest,
) (*receiptv1.ArchiveConversationResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.archiveConversation.Execute(ctx, types.ArchiveConversationCommand{
		AuthContext:    auth,
		ConversationID: types.ConversationID(request.GetConversationId()),
		Archived:       request.GetArchived(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &receiptv1.ArchiveConversationResponse{
		Conversation: conversationSummaryResponse(result.Conversation),
	}, nil
}

func (server *Server) PinConversation(
	ctx context.Context,
	request *receiptv1.PinConversationRequest,
) (*receiptv1.PinConversationResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.pinConversation.Execute(ctx, types.PinConversationCommand{
		AuthContext:    auth,
		ConversationID: types.ConversationID(request.GetConversationId()),
		Pinned:         request.GetPinned(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &receiptv1.PinConversationResponse{
		Conversation: conversationSummaryResponse(result.Conversation),
	}, nil
}

func (server *Server) MuteConversation(
	ctx context.Context,
	request *receiptv1.MuteConversationRequest,
) (*receiptv1.MuteConversationResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.muteConversation.Execute(ctx, types.MuteConversationCommand{
		AuthContext:    auth,
		ConversationID: types.ConversationID(request.GetConversationId()),
		Muted:          request.GetMuted(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &receiptv1.MuteConversationResponse{
		Conversation: conversationSummaryResponse(result.Conversation),
	}, nil
}

func conversationSummaryResponse(item types.ConversationSummary) *receiptv1.ConversationSummary {
	return &receiptv1.ConversationSummary{
		ConversationId:      string(item.ConversationID),
		LastVisibleSeq:      item.LastVisibleSeq,
		LastMessageId:       item.LastMessageID,
		LastSenderId:        string(item.LastSenderID),
		LastSourceEventType: item.LastSourceEventType,
		UnreadCount:         item.UnreadCount,
		LastReadSeq:         item.LastReadSeq,
		UpdatedAtUnixMs:     item.UpdatedAt.UnixMilli(),
		Archived:            item.Archived,
		Pinned:              item.Pinned,
		Muted:               item.Muted,
	}
}

func conversationListSortFromProto(sort receiptv1.ConversationListSort) string {
	switch sort {
	case receiptv1.ConversationListSort_CONVERSATION_LIST_SORT_UNSPECIFIED:
		return ""
	case receiptv1.ConversationListSort_CONVERSATION_LIST_SORT_UPDATED_AT_DESC:
		return types.ConversationListSortUpdatedAtDesc
	case receiptv1.ConversationListSort_CONVERSATION_LIST_SORT_PINNED_UPDATED_AT_DESC:
		return types.ConversationListSortPinnedUpdatedAtDesc
	default:
		return "unsupported"
	}
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
	case errors.Is(err, types.ErrConversationNotFound):
		return status.Error(codes.NotFound, "conversation not found")
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

func authFromProto(ctx context.Context, auth *receiptv1.AuthContext) (types.AuthContext, bool) {
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
