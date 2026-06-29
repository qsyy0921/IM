package grpc

import (
	"context"
	"testing"
	"time"

	timelinev1 "github.com/qsyy0921/IM/api/proto/nexusim/timeline/v1"
	"github.com/qsyy0921/IM/services/timeline-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAllocateSeqBlockExecutor struct {
	command types.AllocateSeqBlockCommand
	result  types.SeqBlockLease
	err     error
}

func (executor *fakeAllocateSeqBlockExecutor) Execute(
	_ context.Context,
	command types.AllocateSeqBlockCommand,
) (types.SeqBlockLease, error) {
	executor.command = command
	return executor.result, executor.err
}

func TestAllocateSeqBlockMapsRequest(t *testing.T) {
	executor := &fakeAllocateSeqBlockExecutor{
		result: types.SeqBlockLease{
			TenantID:         "tenant-a",
			ConversationID:   "conversation-a",
			StartSeq:         10,
			EndSeq:           41,
			BlockSize:        32,
			SequencerEpoch:   1,
			LeaseID:          "seqblk-test",
			ExpiresAt:        time.UnixMilli(1000),
			IdempotentReplay: true,
		},
	}
	server := NewServer(executor)

	response, err := server.AllocateSeqBlock(context.Background(), &timelinev1.AllocateSeqBlockRequest{
		TenantId:       "tenant-a",
		ConversationId: "conversation-a",
		RequesterId:    "message-service-a",
		BlockSize:      32,
		IdempotencyKey: "request-1",
	})
	if err != nil {
		t.Fatalf("allocate seq block: %v", err)
	}
	if executor.command.RequesterID != "message-service-a" || executor.command.BlockSize != 32 {
		t.Fatalf("request was not mapped: %+v", executor.command)
	}
	if response.GetStartSeq() != 10 ||
		response.GetEndSeq() != 41 ||
		!response.GetIdempotentReplay() ||
		response.GetExpiresAtUnixMs() != 1000 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestAllocateSeqBlockMapsIdempotencyConflict(t *testing.T) {
	server := NewServer(&fakeAllocateSeqBlockExecutor{
		err: types.NewIdempotencyConflict("different command"),
	})
	_, err := server.AllocateSeqBlock(context.Background(), &timelinev1.AllocateSeqBlockRequest{
		TenantId:       "tenant-a",
		ConversationId: "conversation-a",
		RequesterId:    "message-service-a",
		BlockSize:      32,
		IdempotencyKey: "request-1",
	})
	if status.Code(err) != codes.Aborted {
		t.Fatalf("expected Aborted, got %v", err)
	}
}
