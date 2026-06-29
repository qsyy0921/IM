package rpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	timelinev1 "github.com/qsyy0921/IM/api/proto/nexusim/timeline/v1"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	timelineRequesterID       = "message-service"
	timelineMetadataTraceID   = "x-nexusim-trace-id"
	timelineMetadataRequestID = "x-nexusim-request-id"
)

type TimelineClient struct {
	client  timelinev1.TimelineServiceClient
	timeout time.Duration
}

type TimelineClientDialConfig struct {
	Addr    string
	Timeout time.Duration
	TLS     TimelineClientTLSConfig
}

type TimelineClientTLSConfig = GRPCClientTLSConfig

func NewTimelineClient(client timelinev1.TimelineServiceClient, timeout time.Duration) TimelineClient {
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}
	return TimelineClient{client: client, timeout: timeout}
}

func DialTimelineClientWithConfig(_ context.Context, config TimelineClientDialConfig) (TimelineClient, func() error, error) {
	addr := strings.TrimSpace(config.Addr)
	if addr == "" {
		return TimelineClient{}, nil, errors.New("timeline service address is required")
	}
	transportCredentials := grpc.WithTransportCredentials(insecure.NewCredentials())
	if config.TLS.Enabled() {
		creds, err := timelineClientTLSCredentials(config.TLS)
		if err != nil {
			return TimelineClient{}, nil, err
		}
		transportCredentials = grpc.WithTransportCredentials(creds)
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		transportCredentials,
	)
	if err != nil {
		return TimelineClient{}, nil, err
	}
	return NewTimelineClient(timelinev1.NewTimelineServiceClient(conn), config.Timeout), conn.Close, nil
}

func timelineClientTLSCredentials(config TimelineClientTLSConfig) (credentials.TransportCredentials, error) {
	return grpcClientTLSCredentials(
		config,
		"NEXUSIM_TIMELINE_SERVICE_TLS_CA_FILE",
		"NEXUSIM_TIMELINE_SERVICE_TLS_CLIENT_CERT_FILE",
		"NEXUSIM_TIMELINE_SERVICE_TLS_CLIENT_KEY_FILE",
	)
}

func (c TimelineClient) AllocateSeqBlock(ctx context.Context, command types.SendMessageCommand, minimumStartSeq int64) (types.SeqBlock, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	callCtx = timelineOutgoingMetadataContext(callCtx, command.AuthContext)

	response, err := c.client.AllocateSeqBlock(callCtx, &timelinev1.AllocateSeqBlockRequest{
		TenantId:        string(command.AuthContext.TenantID),
		ConversationId:  string(command.ConversationID),
		RequesterId:     timelineRequesterID,
		BlockSize:       1,
		IdempotencyKey:  timelineIdempotencyKey(command),
		MinimumStartSeq: minimumStartSeq,
	})
	if err != nil {
		return types.SeqBlock{}, mapTimelineError(err)
	}
	if err := validateTimelineResponse(command, response); err != nil {
		return types.SeqBlock{}, err
	}
	return types.SeqBlock{
		StartSeq: response.GetStartSeq(),
		EndSeq:   response.GetEndSeq(),
		Epoch:    response.GetSequencerEpoch(),
	}, nil
}

func timelineOutgoingMetadataContext(ctx context.Context, auth types.AuthContext) context.Context {
	pairs := make([]string, 0, 4)
	if auth.TraceID != "" {
		pairs = append(pairs, timelineMetadataTraceID, auth.TraceID)
	}
	if auth.RequestID != "" {
		pairs = append(pairs, timelineMetadataRequestID, auth.RequestID)
	}
	if len(pairs) == 0 {
		return ctx
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
}

func timelineIdempotencyKey(command types.SendMessageCommand) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		string(command.AuthContext.TenantID),
		string(command.ConversationID),
		string(command.AuthContext.UserID),
		string(command.AuthContext.DeviceID),
		string(command.ClientMsgID),
	}, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func validateTimelineResponse(command types.SendMessageCommand, response *timelinev1.AllocateSeqBlockResponse) error {
	if response == nil {
		return types.NewSequencerUnavailable("timeline service returned empty response")
	}
	if response.GetTenantId() != string(command.AuthContext.TenantID) ||
		response.GetConversationId() != string(command.ConversationID) {
		return types.NewSequencerUnavailable("timeline service returned mismatched seq block")
	}
	if response.GetStartSeq() <= 0 || response.GetEndSeq() != response.GetStartSeq() || response.GetBlockSize() != 1 {
		return types.NewSequencerUnavailable("timeline service returned invalid single-message seq block")
	}
	if response.GetSequencerEpoch() <= 0 || strings.TrimSpace(response.GetLeaseId()) == "" {
		return types.NewSequencerUnavailable("timeline service returned incomplete seq block lease")
	}
	return nil
}

func mapTimelineError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return types.NewSequencerUnavailable("timeline service deadline exceeded")
	}
	st, ok := status.FromError(err)
	if !ok {
		return types.NewSequencerUnavailable("timeline service error")
	}
	switch st.Code() {
	case codes.Aborted:
		return types.NewIdempotencyConflict("timeline service returned idempotency conflict")
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.NewSequencerUnavailable("timeline service unavailable")
	case codes.InvalidArgument:
		return types.NewSequencerUnavailable("timeline service rejected seq block request")
	default:
		return types.NewSequencerUnavailable(fmt.Sprintf("timeline service returned %s", st.Code()))
	}
}
