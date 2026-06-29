package app

import (
	"context"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/timeline-service/internal/types"
)

type fakeSeqBlockRepository struct {
	command  types.AllocateSeqBlockCommand
	leaseTTL time.Duration
	result   types.SeqBlockLease
	err      error
	called   bool
}

func (repository *fakeSeqBlockRepository) AllocateSeqBlock(
	_ context.Context,
	command types.AllocateSeqBlockCommand,
	leaseTTL time.Duration,
) (types.SeqBlockLease, error) {
	repository.called = true
	repository.command = command
	repository.leaseTTL = leaseTTL
	return repository.result, repository.err
}

func TestAllocateSeqBlockUseCaseValidatesCommand(t *testing.T) {
	repository := &fakeSeqBlockRepository{}
	useCase := NewAllocateSeqBlockUseCase(repository, 10, time.Second)

	_, err := useCase.Execute(context.Background(), types.AllocateSeqBlockCommand{
		TenantID:       "tenant-a",
		ConversationID: "conversation-a",
		RequesterID:    "message-service-a",
		BlockSize:      11,
		IdempotencyKey: "request-1",
	})
	if err == nil {
		t.Fatal("expected block size validation error")
	}
	if repository.called {
		t.Fatal("repository should not be called for invalid command")
	}
}

func TestAllocateSeqBlockUseCaseDelegates(t *testing.T) {
	expected := types.SeqBlockLease{
		TenantID:       "tenant-a",
		ConversationID: "conversation-a",
		StartSeq:       1,
		EndSeq:         32,
		BlockSize:      32,
		SequencerEpoch: 1,
		LeaseID:        "seqblk-test",
		ExpiresAt:      time.Now().Add(time.Minute),
	}
	repository := &fakeSeqBlockRepository{result: expected}
	useCase := NewAllocateSeqBlockUseCase(repository, 100, 30*time.Second)

	result, err := useCase.Execute(context.Background(), types.AllocateSeqBlockCommand{
		TenantID:       "tenant-a",
		ConversationID: "conversation-a",
		RequesterID:    "message-service-a",
		BlockSize:      32,
		IdempotencyKey: "request-1",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !repository.called || repository.leaseTTL != 30*time.Second {
		t.Fatalf("repository delegation mismatch: called=%v ttl=%s", repository.called, repository.leaseTTL)
	}
	if result.StartSeq != expected.StartSeq || result.EndSeq != expected.EndSeq {
		t.Fatalf("unexpected lease result: %+v", result)
	}
}
