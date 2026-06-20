package grpc

import (
	"context"
	"errors"
	"time"

	presencev1 "github.com/qsyy0921/IM/api/proto/nexusim/presence/v1"
	"github.com/qsyy0921/IM/services/presence-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UpdatePresenceExecutor interface {
	Execute(context.Context, types.UpdatePresenceCommand) (types.PresenceState, error)
}

type GetPresenceExecutor interface {
	Execute(context.Context, types.GetPresenceCommand) ([]types.PresenceState, error)
}

type UpdateTypingExecutor interface {
	Execute(context.Context, types.UpdateTypingCommand) (types.TypingIndicator, error)
}

type Server struct {
	presencev1.UnimplementedPresenceServiceServer
	updatePresence UpdatePresenceExecutor
	getPresence    GetPresenceExecutor
	updateTyping   UpdateTypingExecutor
}

func NewServer(
	updatePresence UpdatePresenceExecutor,
	getPresence GetPresenceExecutor,
	updateTyping UpdateTypingExecutor,
) *Server {
	return &Server{
		updatePresence: updatePresence,
		getPresence:    getPresence,
		updateTyping:   updateTyping,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	presencev1.RegisterPresenceServiceServer(registrar, server)
}

func (server *Server) UpdatePresence(
	ctx context.Context,
	request *presencev1.UpdatePresenceRequest,
) (*presencev1.UpdatePresenceResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	state, err := server.updatePresence.Execute(ctx, types.UpdatePresenceCommand{
		AuthContext:    auth,
		UserID:         request.GetUserId(),
		DeviceID:       request.GetDeviceId(),
		SessionID:      request.GetSessionId(),
		PresenceState:  request.GetPresenceState(),
		ManualStatus:   request.GetManualStatus(),
		TTL:            durationFromMillis(request.GetTtlMs()),
		Source:         request.GetSource(),
		IdempotencyKey: request.GetIdempotencyKey(),
		CorrelationID:  request.GetCorrelationId(),
		CausationID:    request.GetCausationId(),
		TraceID:        request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &presencev1.UpdatePresenceResponse{State: stateToProto(state)}, nil
}

func (server *Server) GetPresence(
	ctx context.Context,
	request *presencev1.GetPresenceRequest,
) (*presencev1.GetPresenceResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	states, err := server.getPresence.Execute(ctx, types.GetPresenceCommand{
		AuthContext:     auth,
		RequesterUserID: request.GetRequesterUserId(),
		TargetUserIDs:   request.GetTargetUserIds(),
		ConversationID:  request.GetConversationId(),
		IncludeDevices:  request.GetIncludeDevices(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	response := &presencev1.GetPresenceResponse{States: make([]*presencev1.PresenceState, 0, len(states))}
	for _, state := range states {
		response.States = append(response.States, stateToProto(state))
	}
	return response, nil
}

func (server *Server) UpdateTyping(
	ctx context.Context,
	request *presencev1.UpdateTypingRequest,
) (*presencev1.UpdateTypingResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	typing, err := server.updateTyping.Execute(ctx, types.UpdateTypingCommand{
		AuthContext:    auth,
		ConversationID: request.GetConversationId(),
		UserID:         request.GetUserId(),
		DeviceID:       request.GetDeviceId(),
		TypingState:    request.GetTypingState(),
		TTL:            durationFromMillis(request.GetTtlMs()),
		CorrelationID:  request.GetCorrelationId(),
		CausationID:    request.GetCausationId(),
		TraceID:        request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &presencev1.UpdateTypingResponse{Typing: typingToProto(typing)}, nil
}

func authFromProto(ctx context.Context, auth *presencev1.AuthContext) (types.AuthContext, bool) {
	if verified, ok := verifiedAuthFromContext(ctx); ok {
		return verified, true
	}
	if auth == nil {
		return types.AuthContext{}, false
	}
	return types.AuthContext{
		TenantID:    types.TenantID(auth.GetTenantId()),
		UserID:      auth.GetUserId(),
		ServiceName: auth.GetServiceName(),
		InstanceRef: auth.GetInstanceRef(),
		TraceID:     auth.GetTraceId(),
		RequestID:   auth.GetRequestId(),
	}, true
}

func stateToProto(state types.PresenceState) *presencev1.PresenceState {
	devices := make([]*presencev1.DevicePresence, 0, len(state.DeviceStates))
	for _, device := range state.DeviceStates {
		devices = append(devices, &presencev1.DevicePresence{
			DeviceId:         device.DeviceID,
			SessionId:        device.SessionID,
			State:            device.State,
			DeviceState:      device.DeviceState,
			LastSeenAtUnixMs: timeToUnixMillis(device.LastSeenAt),
			ExpiresAtUnixMs:  timeToUnixMillis(device.ExpiresAt),
		})
	}
	return &presencev1.PresenceState{
		TenantId:           string(state.TenantID),
		UserId:             state.UserID,
		VisibleState:       state.VisibleState,
		ActualState:        state.ActualState,
		ManualStatus:       state.ManualStatus,
		LastSeenAtUnixMs:   timeToUnixMillis(state.LastSeenAt),
		DeviceCount:        int32(state.DeviceCount),
		DeviceStates:       devices,
		VisibilityDecision: state.VisibilityDecision,
	}
}

func typingToProto(typing types.TypingIndicator) *presencev1.TypingIndicator {
	return &presencev1.TypingIndicator{
		TenantId:        string(typing.TenantID),
		ConversationId:  typing.ConversationID,
		UserId:          typing.UserID,
		DeviceId:        typing.DeviceID,
		TypingState:     typing.TypingState,
		ExpiresAtUnixMs: timeToUnixMillis(typing.ExpiresAt),
	}
}

func durationFromMillis(value int64) time.Duration {
	if value <= 0 {
		return 0
	}
	return time.Duration(value) * time.Millisecond
}

func timeToUnixMillis(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UTC().UnixMilli()
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid argument")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, "presence update already exists")
	case errors.Is(err, types.ErrNotFound):
		return status.Error(codes.NotFound, "presence not found")
	case errors.Is(err, types.ErrFailedPrecondition):
		return status.Error(codes.FailedPrecondition, "presence precondition failed")
	case errors.Is(err, types.ErrDBReadFailed):
		return status.Error(codes.Unavailable, "presence read failed")
	case errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "presence write failed")
	default:
		return status.Error(codes.Internal, "presence internal error")
	}
}
