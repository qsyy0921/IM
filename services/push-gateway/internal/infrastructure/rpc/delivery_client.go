package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	deliveryMetadataTenantID  = "x-nexusim-tenant-id"
	deliveryMetadataUserID    = "x-nexusim-user-id"
	deliveryMetadataDeviceID  = "x-nexusim-device-id"
	deliveryMetadataSessionID = "x-nexusim-session-id"
	deliveryMetadataTraceID   = "x-nexusim-trace-id"
	deliveryMetadataRequestID = "x-nexusim-request-id"

	maxDeliveryCorrelationMetadataLength = 128
)

type DeliveryClient struct {
	client  deliveryv1.DeliveryServiceClient
	timeout time.Duration
}

type DeliveryClientDialConfig struct {
	Addr    string
	Timeout time.Duration
	TLS     DeliveryClientTLSConfig
}

type DeliveryClientTLSConfig = GRPCClientTLSConfig

func NewDeliveryClient(client deliveryv1.DeliveryServiceClient, timeout time.Duration) DeliveryClient {
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}
	return DeliveryClient{client: client, timeout: timeout}
}

func DialDeliveryClient(ctx context.Context, addr string, timeout time.Duration) (DeliveryClient, func() error, error) {
	return DialDeliveryClientWithConfig(ctx, DeliveryClientDialConfig{Addr: addr, Timeout: timeout})
}

func DialDeliveryClientWithConfig(_ context.Context, config DeliveryClientDialConfig) (DeliveryClient, func() error, error) {
	addr := strings.TrimSpace(config.Addr)
	if addr == "" {
		return DeliveryClient{}, nil, errors.New("delivery service address is required")
	}
	transportCredentials := grpc.WithTransportCredentials(insecure.NewCredentials())
	if config.TLS.Enabled() {
		creds, err := deliveryClientTLSCredentials(config.TLS)
		if err != nil {
			return DeliveryClient{}, nil, err
		}
		transportCredentials = grpc.WithTransportCredentials(creds)
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		transportCredentials,
	)
	if err != nil {
		return DeliveryClient{}, nil, err
	}
	return NewDeliveryClient(deliveryv1.NewDeliveryServiceClient(conn), config.Timeout), conn.Close, nil
}

func deliveryClientTLSCredentials(config DeliveryClientTLSConfig) (credentials.TransportCredentials, error) {
	return grpcClientTLSCredentials(
		config,
		"NEXUSIM_DELIVERY_SERVICE_TLS_CA_FILE",
		"NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_CERT_FILE",
		"NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_KEY_FILE",
	)
}

func (client DeliveryClient) AckDelivery(
	ctx context.Context,
	command types.AckDeliveryCommand,
) (types.AckDeliveryResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	auth := sanitizeDeliveryCorrelation(command.AuthContext)
	callCtx = deliveryOutgoingMetadataContext(callCtx, auth)

	response, err := client.client.AckDelivery(callCtx, &deliveryv1.AckDeliveryRequest{
		AuthContext: &deliveryv1.AuthContext{
			TenantId:  auth.TenantID,
			UserId:    auth.UserID,
			DeviceId:  auth.DeviceID,
			SessionId: auth.SessionID,
			TraceId:   auth.TraceID,
			RequestId: auth.RequestID,
		},
		ConversationId: command.ConversationID,
		ReceivedSeq:    command.ReceivedSeq,
	})
	if err != nil {
		return types.AckDeliveryResult{}, mapDeliveryError(err)
	}
	if response.GetTenantId() != command.AuthContext.TenantID ||
		response.GetUserId() != command.AuthContext.UserID ||
		response.GetDeviceId() != command.AuthContext.DeviceID ||
		response.GetConversationId() != command.ConversationID {
		return types.AckDeliveryResult{}, types.NewDeliveryUnavailable("delivery service returned mismatched ack")
	}
	return types.AckDeliveryResult{
		TenantID:        response.GetTenantId(),
		UserID:          response.GetUserId(),
		DeviceID:        response.GetDeviceId(),
		ConversationID:  response.GetConversationId(),
		LastReceivedSeq: response.GetLastReceivedSeq(),
	}, nil
}

func mapDeliveryError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return types.NewDeliveryUnavailable("delivery service deadline exceeded")
	}
	st, ok := status.FromError(err)
	if !ok {
		return types.NewDeliveryUnavailable("delivery service error")
	}
	switch st.Code() {
	case codes.FailedPrecondition:
		return types.NewAckOutOfVisibleRange("delivery service rejected ack")
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.NewDeliveryUnavailable("delivery service unavailable")
	case codes.InvalidArgument:
		return types.NewInvalidFrame("delivery ack invalid")
	case codes.PermissionDenied:
		return types.ErrPermissionDenied
	default:
		return types.NewDeliveryUnavailable("delivery service error")
	}
}

func deliveryOutgoingMetadataContext(ctx context.Context, auth types.AuthContext) context.Context {
	pairs := []string{
		deliveryMetadataTenantID, auth.TenantID,
		deliveryMetadataUserID, auth.UserID,
		deliveryMetadataDeviceID, auth.DeviceID,
	}
	if auth.SessionID != "" {
		pairs = append(pairs, deliveryMetadataSessionID, auth.SessionID)
	}
	if auth.TraceID != "" {
		pairs = append(pairs, deliveryMetadataTraceID, auth.TraceID)
	}
	if auth.RequestID != "" {
		pairs = append(pairs, deliveryMetadataRequestID, auth.RequestID)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}

func sanitizeDeliveryCorrelation(auth types.AuthContext) types.AuthContext {
	auth.TraceID = sanitizeDeliveryCorrelationValue(auth.TraceID)
	auth.RequestID = sanitizeDeliveryCorrelationValue(auth.RequestID)
	return auth
}

func sanitizeDeliveryCorrelationValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) > maxDeliveryCorrelationMetadataLength {
		runes = runes[:maxDeliveryCorrelationMetadataLength]
	}
	for _, r := range runes {
		if isDeliveryCorrelationRune(r) {
			continue
		}
		return ""
	}
	return string(runes)
}

func isDeliveryCorrelationRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' ||
		r == '_' ||
		r == '.' ||
		r == ':'
}
