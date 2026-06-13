package rpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	conversationMetadataTraceID   = "x-nexusim-trace-id"
	conversationMetadataRequestID = "x-nexusim-request-id"
)

type ConversationClient struct {
	client  conversationv1.ConversationServiceClient
	timeout time.Duration
}

type ConversationClientDialConfig struct {
	Addr    string
	Timeout time.Duration
	TLS     ConversationClientTLSConfig
}

type ConversationClientTLSConfig = GRPCClientTLSConfig

func NewConversationClient(client conversationv1.ConversationServiceClient, timeout time.Duration) ConversationClient {
	if timeout <= 0 {
		timeout = 30 * time.Millisecond
	}
	return ConversationClient{client: client, timeout: timeout}
}

func DialConversationClient(
	ctx context.Context,
	addr string,
	timeout time.Duration,
) (ConversationClient, func() error, error) {
	return DialConversationClientWithConfig(ctx, ConversationClientDialConfig{Addr: addr, Timeout: timeout})
}

func DialConversationClientWithConfig(_ context.Context, config ConversationClientDialConfig) (ConversationClient, func() error, error) {
	addr := strings.TrimSpace(config.Addr)
	if addr == "" {
		return ConversationClient{}, nil, errors.New("conversation service address is required")
	}
	transportCredentials := grpc.WithTransportCredentials(insecure.NewCredentials())
	if config.TLS.Enabled() {
		creds, err := conversationClientTLSCredentials(config.TLS)
		if err != nil {
			return ConversationClient{}, nil, err
		}
		transportCredentials = grpc.WithTransportCredentials(creds)
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		transportCredentials,
	)
	if err != nil {
		return ConversationClient{}, nil, err
	}
	return NewConversationClient(conversationv1.NewConversationServiceClient(conn), config.Timeout), conn.Close, nil
}

func conversationClientTLSCredentials(config ConversationClientTLSConfig) (credentials.TransportCredentials, error) {
	return grpcClientTLSCredentials(
		config,
		"NEXUSIM_CONVERSATION_SERVICE_TLS_CA_FILE",
		"NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_CERT_FILE",
		"NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_KEY_FILE",
	)
}

func (c ConversationClient) GetSendContext(
	ctx context.Context,
	command types.SendMessageCommand,
) (types.ConversationSendContext, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	callCtx = conversationOutgoingMetadataContext(callCtx, command.AuthContext)

	response, err := c.client.GetSendContext(callCtx, &conversationv1.GetSendContextRequest{
		TenantId:       string(command.AuthContext.TenantID),
		ConversationId: string(command.ConversationID),
		UserId:         string(command.AuthContext.UserID),
		TraceId:        command.AuthContext.TraceID,
	})
	if err != nil {
		return types.ConversationSendContext{}, mapConversationError(err)
	}
	if err := validateConversationResponse(command, response); err != nil {
		return types.ConversationSendContext{}, err
	}
	return types.ConversationSendContext{
		MemberVersion:       response.GetMemberVersion(),
		PermissionVersion:   response.GetPermissionVersion(),
		ConversationMode:    fromProtoConversationMode(response.GetConversationMode()),
		FanoutMode:          fromProtoFanoutMode(response.GetFanoutMode()),
		FanoutPolicyVersion: response.GetFanoutPolicyVersion(),
		CurrentSeqShard:     response.GetCurrentSeqShard(),
		DirectPeerUserID:    types.UserID(response.GetDirectPeerUserId()),
	}, nil
}

func conversationOutgoingMetadataContext(ctx context.Context, auth types.AuthContext) context.Context {
	pairs := make([]string, 0, 4)
	if auth.TraceID != "" {
		pairs = append(pairs, conversationMetadataTraceID, auth.TraceID)
	}
	if auth.RequestID != "" {
		pairs = append(pairs, conversationMetadataRequestID, auth.RequestID)
	}
	if len(pairs) == 0 {
		return ctx
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
}

func validateConversationResponse(command types.SendMessageCommand, response *conversationv1.GetSendContextResponse) error {
	if response == nil {
		return types.NewDependencyUnavailable("conversation service returned empty response")
	}
	if response.GetTenantId() != string(command.AuthContext.TenantID) ||
		response.GetConversationId() != string(command.ConversationID) {
		return types.NewDependencyUnavailable("conversation service returned mismatched context")
	}
	if fromProtoConversationMode(response.GetConversationMode()) == "" {
		return types.NewDependencyUnavailable("conversation service returned invalid conversation mode")
	}
	if fromProtoFanoutMode(response.GetFanoutMode()) == "" {
		return types.NewDependencyUnavailable("conversation service returned invalid fanout mode")
	}
	if response.GetCurrentSeqShard() == "" {
		return types.NewDependencyUnavailable("conversation service returned empty seq shard")
	}
	return nil
}

func mapConversationError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return types.NewDependencyUnavailable("conversation service deadline exceeded")
	}
	st, ok := status.FromError(err)
	if !ok {
		return types.NewDependencyUnavailable("conversation service error")
	}
	switch st.Code() {
	case codes.NotFound:
		return types.NewConversationNotFound("conversation service returned not found")
	case codes.PermissionDenied:
		return types.NewPermissionDenied("conversation member is not active")
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.NewDependencyUnavailable("conversation service unavailable")
	default:
		return types.NewDependencyUnavailable(fmt.Sprintf("conversation service returned %s", st.Code()))
	}
}

func fromProtoConversationMode(mode conversationv1.ConversationMode) types.ConversationMode {
	switch mode {
	case conversationv1.ConversationMode_CONVERSATION_MODE_LOCAL_ROW_LOCK:
		return types.ConversationModeLocalRowLock
	case conversationv1.ConversationMode_CONVERSATION_MODE_SEQUENCER_BLOCK:
		return types.ConversationModeSequencerBlock
	default:
		return ""
	}
}

func fromProtoFanoutMode(mode conversationv1.FanoutMode) types.FanoutMode {
	switch mode {
	case conversationv1.FanoutMode_FANOUT_MODE_WRITE_FANOUT:
		return types.FanoutModeWriteFanout
	case conversationv1.FanoutMode_FANOUT_MODE_HYBRID_FANOUT:
		return types.FanoutModeHybridFanout
	case conversationv1.FanoutMode_FANOUT_MODE_READ_FANOUT:
		return types.FanoutModeReadFanout
	case conversationv1.FanoutMode_FANOUT_MODE_BROADCAST_SIGNAL:
		return types.FanoutModeBroadcastSignal
	default:
		return ""
	}
}
