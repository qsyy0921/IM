package rpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	client            timelinev1.TimelineServiceClient
	timeout           time.Duration
	blockSize         int
	leaseSafetyMargin time.Duration
	cache             *timelineSeqBlockCache
}

type TimelineClientDialConfig struct {
	Addr              string
	Timeout           time.Duration
	BlockSize         int
	LeaseSafetyMargin time.Duration
	TLS               TimelineClientTLSConfig
}

type TimelineClientTLSConfig = GRPCClientTLSConfig

func NewTimelineClient(client timelinev1.TimelineServiceClient, timeout time.Duration) TimelineClient {
	return NewTimelineClientWithConfig(client, timeout, 1, 0)
}

func NewTimelineClientWithConfig(
	client timelinev1.TimelineServiceClient,
	timeout time.Duration,
	blockSize int,
	leaseSafetyMargin time.Duration,
) TimelineClient {
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}
	if blockSize <= 0 {
		blockSize = 1
	}
	if leaseSafetyMargin < 0 {
		leaseSafetyMargin = 0
	}
	return TimelineClient{
		client:            client,
		timeout:           timeout,
		blockSize:         blockSize,
		leaseSafetyMargin: leaseSafetyMargin,
		cache:             newTimelineSeqBlockCache(),
	}
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
	return NewTimelineClientWithConfig(
		timelinev1.NewTimelineServiceClient(conn),
		config.Timeout,
		config.BlockSize,
		config.LeaseSafetyMargin,
	), conn.Close, nil
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
	if c.cache != nil && c.blockSize > 1 {
		return c.cache.next(ctx, c, command, minimumStartSeq)
	}
	return c.allocateRemoteSeqBlock(ctx, command, minimumStartSeq, 1, timelineIdempotencyKey(command))
}

func (c TimelineClient) allocateRemoteSeqBlock(
	ctx context.Context,
	command types.SendMessageCommand,
	minimumStartSeq int64,
	blockSize int,
	idempotencyKey string,
) (types.SeqBlock, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	callCtx = timelineOutgoingMetadataContext(callCtx, command.AuthContext)

	response, err := c.client.AllocateSeqBlock(callCtx, &timelinev1.AllocateSeqBlockRequest{
		TenantId:        string(command.AuthContext.TenantID),
		ConversationId:  string(command.ConversationID),
		RequesterId:     timelineRequesterID,
		BlockSize:       int32(blockSize),
		IdempotencyKey:  idempotencyKey,
		MinimumStartSeq: minimumStartSeq,
	})
	if err != nil {
		return types.SeqBlock{}, mapTimelineError(err)
	}
	if err := validateTimelineResponse(command, response, blockSize); err != nil {
		return types.SeqBlock{}, err
	}
	return types.SeqBlock{
		StartSeq:  response.GetStartSeq(),
		EndSeq:    response.GetEndSeq(),
		Epoch:     response.GetSequencerEpoch(),
		LeaseID:   response.GetLeaseId(),
		ExpiresAt: time.UnixMilli(response.GetExpiresAtUnixMs()).UTC(),
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

func validateTimelineResponse(command types.SendMessageCommand, response *timelinev1.AllocateSeqBlockResponse, requestedBlockSize int) error {
	if response == nil {
		return types.NewSequencerUnavailable("timeline service returned empty response")
	}
	if response.GetTenantId() != string(command.AuthContext.TenantID) ||
		response.GetConversationId() != string(command.ConversationID) {
		return types.NewSequencerUnavailable("timeline service returned mismatched seq block")
	}
	if response.GetStartSeq() <= 0 || response.GetEndSeq() < response.GetStartSeq() || int(response.GetBlockSize()) != requestedBlockSize {
		return types.NewSequencerUnavailable("timeline service returned invalid seq block")
	}
	if response.GetSequencerEpoch() <= 0 || strings.TrimSpace(response.GetLeaseId()) == "" {
		return types.NewSequencerUnavailable("timeline service returned incomplete seq block lease")
	}
	if response.GetExpiresAtUnixMs() <= 0 {
		return types.NewSequencerUnavailable("timeline service returned missing lease expiry")
	}
	return nil
}

type timelineSeqBlockCache struct {
	mu     sync.Mutex
	blocks map[string]timelineCachedSeqBlock
}

type timelineCachedSeqBlock struct {
	nextSeq   int64
	endSeq    int64
	epoch     int64
	leaseID   string
	expiresAt time.Time
}

func newTimelineSeqBlockCache() *timelineSeqBlockCache {
	return &timelineSeqBlockCache{blocks: map[string]timelineCachedSeqBlock{}}
}

func (cache *timelineSeqBlockCache) next(
	ctx context.Context,
	client TimelineClient,
	command types.SendMessageCommand,
	minimumStartSeq int64,
) (types.SeqBlock, error) {
	key := timelineCacheKey(command)
	cache.mu.Lock()
	defer cache.mu.Unlock()

	now := time.Now().UTC()
	if block, ok := cache.blocks[key]; ok && block.usable(now, client.leaseSafetyMargin, minimumStartSeq) {
		seq := block.nextSeq
		block.nextSeq++
		cache.blocks[key] = block
		return types.SeqBlock{
			StartSeq:  seq,
			EndSeq:    seq,
			Epoch:     block.epoch,
			LeaseID:   block.leaseID,
			ExpiresAt: block.expiresAt,
		}, nil
	}

	blockID := timelineBlockIdempotencyKey(command, minimumStartSeq)
	lease, err := client.allocateRemoteSeqBlock(ctx, command, minimumStartSeq, client.blockSize, blockID)
	if err != nil {
		delete(cache.blocks, key)
		return types.SeqBlock{}, err
	}
	if lease.EndSeq < lease.StartSeq {
		delete(cache.blocks, key)
		return types.SeqBlock{}, types.NewSequencerUnavailable("timeline service returned invalid cached seq block")
	}

	nextSeq := lease.StartSeq + 1
	if nextSeq <= lease.EndSeq {
		cache.blocks[key] = timelineCachedSeqBlock{
			nextSeq:   nextSeq,
			endSeq:    lease.EndSeq,
			epoch:     lease.Epoch,
			leaseID:   lease.LeaseID,
			expiresAt: lease.ExpiresAt,
		}
	} else {
		delete(cache.blocks, key)
	}
	return types.SeqBlock{
		StartSeq:  lease.StartSeq,
		EndSeq:    lease.StartSeq,
		Epoch:     lease.Epoch,
		LeaseID:   lease.LeaseID,
		ExpiresAt: lease.ExpiresAt,
	}, nil
}

func (block timelineCachedSeqBlock) usable(now time.Time, safetyMargin time.Duration, minimumStartSeq int64) bool {
	if block.nextSeq <= 0 || block.nextSeq > block.endSeq {
		return false
	}
	if minimumStartSeq > 0 && block.nextSeq < minimumStartSeq {
		return false
	}
	return now.Add(safetyMargin).Before(block.expiresAt)
}

func timelineCacheKey(command types.SendMessageCommand) string {
	return strings.Join([]string{
		string(command.AuthContext.TenantID),
		string(command.ConversationID),
	}, "\x1f")
}

func timelineBlockIdempotencyKey(command types.SendMessageCommand, minimumStartSeq int64) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		"block",
		string(command.AuthContext.TenantID),
		string(command.ConversationID),
		fmt.Sprintf("%d", minimumStartSeq),
		fmt.Sprintf("%d", time.Now().UnixNano()),
	}, "\x1f")))
	return hex.EncodeToString(sum[:])
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
