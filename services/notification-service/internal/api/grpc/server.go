package grpc

import (
	"context"
	"errors"
	"time"

	notificationv1 "github.com/qsyy0921/IM/api/proto/nexusim/notification/v1"
	"github.com/qsyy0921/IM/services/notification-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CreateNotificationRequestExecutor interface {
	Execute(context.Context, types.CreateNotificationRequestCommand) (types.NotificationRequest, error)
}

type GetNotificationStatusExecutor interface {
	Execute(context.Context, types.GetNotificationStatusCommand) (types.NotificationRequest, error)
}

type CancelNotificationRequestExecutor interface {
	Execute(context.Context, types.CancelNotificationRequestCommand) (types.NotificationRequest, error)
}

type Server struct {
	notificationv1.UnimplementedNotificationServiceServer
	createNotificationRequest CreateNotificationRequestExecutor
	getNotificationStatus     GetNotificationStatusExecutor
	cancelNotificationRequest CancelNotificationRequestExecutor
}

func NewServer(
	createNotificationRequest CreateNotificationRequestExecutor,
	getNotificationStatus GetNotificationStatusExecutor,
	cancelNotificationRequest CancelNotificationRequestExecutor,
) *Server {
	return &Server{
		createNotificationRequest: createNotificationRequest,
		getNotificationStatus:     getNotificationStatus,
		cancelNotificationRequest: cancelNotificationRequest,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	notificationv1.RegisterNotificationServiceServer(registrar, server)
}

func (server *Server) CreateNotificationRequest(
	ctx context.Context,
	request *notificationv1.CreateNotificationRequestRequest,
) (*notificationv1.CreateNotificationRequestResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.createNotificationRequest.Execute(ctx, types.CreateNotificationRequestCommand{
		AuthContext:             auth,
		RequesterService:        request.GetRequesterService(),
		RequesterUserID:         types.UserID(request.GetRequesterUserId()),
		Channel:                 channelFromProto(request.GetChannel()),
		RecipientRef:            request.GetRecipientRef(),
		DestinationRef:          request.GetDestinationRef(),
		DestinationMasked:       request.GetDestinationMasked(),
		TemplateKey:             request.GetTemplateKey(),
		TemplateVersion:         request.GetTemplateVersion(),
		Locale:                  request.GetLocale(),
		Priority:                priorityFromProto(request.GetPriority()),
		ScheduledAt:             unixMillisToTime(request.GetScheduledAtUnixMs()),
		ExpiresAt:               unixMillisToTime(request.GetExpiresAtUnixMs()),
		IdempotencyKey:          request.GetIdempotencyKey(),
		TemplateVariablesJSON:   request.GetTemplateVariablesJson(),
		SecretPayloadCiphertext: request.GetSecretPayloadCiphertext(),
		SecretPayloadKeyVersion: request.GetSecretPayloadKeyVersion(),
		SecretPayloadExpiresAt:  unixMillisToTime(request.GetSecretPayloadExpiresAtUnixMs()),
		CorrelationID:           request.GetCorrelationId(),
		CausationID:             request.GetCausationId(),
		TraceID:                 request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &notificationv1.CreateNotificationRequestResponse{
		RequestId:           result.RequestID,
		Status:              statusToProto(result.Status),
		NextAttemptAtUnixMs: timeToUnixMillis(result.NextAttemptAt),
		AcceptedAtUnixMs:    timeToUnixMillis(result.CreatedAt),
		Record:              requestToProto(result),
	}, nil
}

func (server *Server) GetNotificationStatus(
	ctx context.Context,
	request *notificationv1.GetNotificationStatusRequest,
) (*notificationv1.GetNotificationStatusResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.getNotificationStatus.Execute(ctx, types.GetNotificationStatusCommand{
		AuthContext: auth,
		RequestID:   request.GetRequestId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &notificationv1.GetNotificationStatusResponse{Record: requestToProto(result)}, nil
}

func (server *Server) CancelNotificationRequest(
	ctx context.Context,
	request *notificationv1.CancelNotificationRequestRequest,
) (*notificationv1.CancelNotificationRequestResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.cancelNotificationRequest.Execute(ctx, types.CancelNotificationRequestCommand{
		AuthContext:     auth,
		RequestID:       request.GetRequestId(),
		CancelRequestID: request.GetCancelRequestId(),
		Reason:          request.GetReason(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &notificationv1.CancelNotificationRequestResponse{Record: requestToProto(result)}, nil
}

func authFromProto(ctx context.Context, auth *notificationv1.AuthContext) (types.AuthContext, bool) {
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

func requestToProto(request types.NotificationRequest) *notificationv1.NotificationRequestRecord {
	return &notificationv1.NotificationRequestRecord{
		TenantId:             string(request.TenantID),
		RequestId:            request.RequestID,
		RequesterService:     request.RequesterService,
		RequesterUserId:      string(request.RequesterUserID),
		Channel:              channelToProto(request.Channel),
		RecipientRef:         request.RecipientRef,
		DestinationMasked:    request.DestinationMasked,
		TemplateKey:          request.TemplateKey,
		TemplateVersion:      request.TemplateVersion,
		Locale:               request.Locale,
		Priority:             priorityToProto(request.Priority),
		Status:               statusToProto(request.Status),
		AttemptCount:         int32(request.AttemptCount),
		NextAttemptAtUnixMs:  timeToUnixMillis(request.NextAttemptAt),
		ExpiresAtUnixMs:      timeToUnixMillis(request.ExpiresAt),
		LastFailureClass:     request.LastFailureClass,
		LastPublicError:      request.LastPublicError,
		CreatedAtUnixMs:      timeToUnixMillis(request.CreatedAt),
		DeliveredAtUnixMs:    timeToUnixMillis(request.DeliveredAt),
		DeadLetteredAtUnixMs: timeToUnixMillis(request.DeadLetteredAt),
		CanceledAtUnixMs:     timeToUnixMillis(request.CanceledAt),
		CorrelationId:        request.CorrelationID,
		CausationId:          request.CausationID,
		TraceId:              request.TraceID,
	}
}

func channelFromProto(channel notificationv1.NotificationChannel) string {
	switch channel {
	case notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_EMAIL:
		return types.ChannelEmail
	case notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_SMS:
		return types.ChannelSMS
	case notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_APNS:
		return types.ChannelAPNS
	case notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_FCM:
		return types.ChannelFCM
	case notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_SYSTEM:
		return types.ChannelSystem
	default:
		return ""
	}
}

func channelToProto(channel string) notificationv1.NotificationChannel {
	switch channel {
	case types.ChannelEmail:
		return notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_EMAIL
	case types.ChannelSMS:
		return notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_SMS
	case types.ChannelAPNS:
		return notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_APNS
	case types.ChannelFCM:
		return notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_FCM
	case types.ChannelSystem:
		return notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_SYSTEM
	default:
		return notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_UNSPECIFIED
	}
}

func priorityFromProto(priority notificationv1.NotificationPriority) string {
	switch priority {
	case notificationv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW:
		return types.PriorityLow
	case notificationv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH:
		return types.PriorityHigh
	case notificationv1.NotificationPriority_NOTIFICATION_PRIORITY_NORMAL:
		return types.PriorityNormal
	default:
		return ""
	}
}

func priorityToProto(priority string) notificationv1.NotificationPriority {
	switch priority {
	case types.PriorityLow:
		return notificationv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW
	case types.PriorityHigh:
		return notificationv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
	case types.PriorityNormal:
		return notificationv1.NotificationPriority_NOTIFICATION_PRIORITY_NORMAL
	default:
		return notificationv1.NotificationPriority_NOTIFICATION_PRIORITY_UNSPECIFIED
	}
}

func statusToProto(status string) notificationv1.NotificationRequestStatus {
	switch status {
	case types.StatusAccepted:
		return notificationv1.NotificationRequestStatus_NOTIFICATION_REQUEST_STATUS_ACCEPTED
	case types.StatusScheduled:
		return notificationv1.NotificationRequestStatus_NOTIFICATION_REQUEST_STATUS_SCHEDULED
	case types.StatusSending:
		return notificationv1.NotificationRequestStatus_NOTIFICATION_REQUEST_STATUS_SENDING
	case types.StatusRetryWait:
		return notificationv1.NotificationRequestStatus_NOTIFICATION_REQUEST_STATUS_RETRY_WAIT
	case types.StatusDelivered:
		return notificationv1.NotificationRequestStatus_NOTIFICATION_REQUEST_STATUS_DELIVERED
	case types.StatusDLQ:
		return notificationv1.NotificationRequestStatus_NOTIFICATION_REQUEST_STATUS_DLQ
	case types.StatusCanceled:
		return notificationv1.NotificationRequestStatus_NOTIFICATION_REQUEST_STATUS_CANCELED
	default:
		return notificationv1.NotificationRequestStatus_NOTIFICATION_REQUEST_STATUS_UNSPECIFIED
	}
}

func unixMillisToTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
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
		return status.Error(codes.AlreadyExists, "notification request already exists")
	case errors.Is(err, types.ErrNotFound):
		return status.Error(codes.NotFound, "notification request not found")
	case errors.Is(err, types.ErrFailedPrecondition):
		return status.Error(codes.FailedPrecondition, "notification request precondition failed")
	case errors.Is(err, types.ErrDBReadFailed):
		return status.Error(codes.Unavailable, "notification read failed")
	case errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "notification write failed")
	case errors.Is(err, types.ErrDependencyFailed):
		return status.Error(codes.Unavailable, "notification dependency failed")
	default:
		return status.Error(codes.Internal, "notification internal error")
	}
}
