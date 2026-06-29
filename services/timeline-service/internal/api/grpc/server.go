package grpc

import (
	"context"
	"errors"

	timelinev1 "github.com/qsyy0921/IM/api/proto/nexusim/timeline/v1"
	"github.com/qsyy0921/IM/services/timeline-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AllocateSeqBlockExecutor interface {
	Execute(context.Context, types.AllocateSeqBlockCommand) (types.SeqBlockLease, error)
}

type Server struct {
	timelinev1.UnimplementedTimelineServiceServer
	allocateSeqBlock AllocateSeqBlockExecutor
}

func NewServer(allocateSeqBlock AllocateSeqBlockExecutor) *Server {
	return &Server{allocateSeqBlock: allocateSeqBlock}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	timelinev1.RegisterTimelineServiceServer(registrar, server)
}

func (server *Server) AllocateSeqBlock(
	ctx context.Context,
	request *timelinev1.AllocateSeqBlockRequest,
) (*timelinev1.AllocateSeqBlockResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if server.allocateSeqBlock == nil {
		return nil, status.Error(codes.Unimplemented, "seq block allocator is not configured")
	}
	result, err := server.allocateSeqBlock.Execute(ctx, types.AllocateSeqBlockCommand{
		TenantID:        request.GetTenantId(),
		ConversationID:  request.GetConversationId(),
		RequesterID:     request.GetRequesterId(),
		BlockSize:       int(request.GetBlockSize()),
		IdempotencyKey:  request.GetIdempotencyKey(),
		MinimumStartSeq: request.GetMinimumStartSeq(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &timelinev1.AllocateSeqBlockResponse{
		TenantId:         result.TenantID,
		ConversationId:   result.ConversationID,
		StartSeq:         result.StartSeq,
		EndSeq:           result.EndSeq,
		BlockSize:        int32(result.BlockSize),
		SequencerEpoch:   result.SequencerEpoch,
		LeaseId:          result.LeaseID,
		ExpiresAtUnixMs:  result.ExpiresAt.UnixMilli(),
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid argument")
	case errors.Is(err, types.ErrIdempotencyConflict):
		return status.Error(codes.Aborted, "idempotency conflict")
	case errors.Is(err, types.ErrDBReadFailed), errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Internal, "timeline storage error")
	default:
		return status.Error(codes.Internal, "timeline internal error")
	}
}
