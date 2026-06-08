package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func TestSendMessageUseCaseSuccess(t *testing.T) {
	repo := &fakeMessageRepository{
		result: domain.AppendMessageResult{
			MessageID:       "msg-1",
			ConversationSeq: 1,
			AcceptedAt:      time.Unix(100, 0).UTC(),
		},
	}
	useCase := newTestUseCase(repo)

	result, err := useCase.Execute(context.Background(), testCommand())
	if err != nil {
		t.Fatalf("execute send message: %v", err)
	}
	if result.MessageID != "msg-1" || result.ConversationSeq != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if repo.calls != 1 {
		t.Fatalf("expected repository called once, got %d", repo.calls)
	}
	if repo.input.Command.ClientMsgID != "client-1" ||
		repo.input.Permission.PermissionVersion != 7 ||
		repo.input.Conversation.PermissionVersion != 7 {
		t.Fatalf("unexpected repository input: %+v", repo.input)
	}
}

func TestSendMessageUseCasePermissionDenied(t *testing.T) {
	repo := &fakeMessageRepository{}
	useCase := NewSendMessageUseCase(
		&fakePolicy{decision: types.PermissionDecision{Allowed: false, Reason: "blocked"}},
		&fakeConversation{context: localConversation()},
		fakeSequencer{},
		repo,
	)

	_, err := useCase.Execute(context.Background(), testCommand())
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
	if repo.calls != 0 {
		t.Fatalf("repository should not be called")
	}
}

func TestSendMessageUseCaseAdmissionRejectsBeforeDependencyReads(t *testing.T) {
	repo := &fakeMessageRepository{}
	policy := &fakePolicy{decision: allowedDecision()}
	conversation := &fakeConversation{context: localConversation()}
	useCase := NewSendMessageUseCase(
		policy,
		conversation,
		fakeSequencer{},
		repo,
		WithAdmission(&fakeAdmission{err: types.NewServiceOverloaded("test overload")}),
	)

	_, err := useCase.Execute(context.Background(), testCommand())
	if !errors.Is(err, types.ErrServiceOverloaded) {
		t.Fatalf("expected service overloaded, got %v", err)
	}
	if policy.calls != 0 || conversation.calls != 0 || repo.calls != 0 {
		t.Fatalf("admission should reject before dependency reads: policy=%d conversation=%d repo=%d", policy.calls, conversation.calls, repo.calls)
	}
}

func TestSendMessageUseCaseRejectsSequencerModeInPhaseOne(t *testing.T) {
	repo := &fakeMessageRepository{}
	conversation := localConversation()
	conversation.ConversationMode = types.ConversationModeSequencerBlock
	useCase := NewSendMessageUseCase(
		&fakePolicy{decision: allowedDecision()},
		&fakeConversation{context: conversation},
		fakeSequencer{},
		repo,
	)

	_, err := useCase.Execute(context.Background(), testCommand())
	if !errors.Is(err, types.ErrSequencerUnavailable) {
		t.Fatalf("expected sequencer unavailable, got %v", err)
	}
	if repo.calls != 0 {
		t.Fatalf("repository should not be called")
	}
}

func TestSendMessageUseCasePropagatesIdempotentReplay(t *testing.T) {
	repo := &fakeMessageRepository{
		result: domain.AppendMessageResult{
			MessageID:        "msg-1",
			ConversationSeq:  1,
			AcceptedAt:       time.Unix(100, 0).UTC(),
			IdempotentReplay: true,
		},
	}
	useCase := newTestUseCase(repo)

	result, err := useCase.Execute(context.Background(), testCommand())
	if err != nil {
		t.Fatalf("execute send message: %v", err)
	}
	if !result.IdempotentReplay {
		t.Fatalf("expected idempotent replay")
	}
}

func TestSendMessageUseCaseRetriesPermissionVersionMismatchOnce(t *testing.T) {
	repo := &fakeMessageRepository{
		result: domain.AppendMessageResult{
			MessageID:       "msg-1",
			ConversationSeq: 1,
			AcceptedAt:      time.Unix(100, 0).UTC(),
		},
	}
	firstConversation := localConversation()
	firstConversation.PermissionVersion = 7
	secondConversation := localConversation()
	secondConversation.PermissionVersion = 8
	firstDecision := allowedDecision()
	firstDecision.PermissionVersion = 8
	secondDecision := allowedDecision()
	secondDecision.PermissionVersion = 8
	policy := &fakePolicy{decisions: []types.PermissionDecision{firstDecision, secondDecision}}
	conversation := &fakeConversation{contexts: []types.ConversationSendContext{firstConversation, secondConversation}}
	useCase := NewSendMessageUseCase(policy, conversation, fakeSequencer{}, repo)

	_, err := useCase.Execute(context.Background(), testCommand())
	if err != nil {
		t.Fatalf("execute send message: %v", err)
	}
	if policy.calls != 2 || conversation.calls != 2 {
		t.Fatalf("expected one retry, policy calls=%d conversation calls=%d", policy.calls, conversation.calls)
	}
	if repo.calls != 1 || repo.input.Permission.PermissionVersion != 8 || repo.input.Conversation.PermissionVersion != 8 {
		t.Fatalf("unexpected repository call after retry: calls=%d input=%+v", repo.calls, repo.input)
	}
}

func TestSendMessageUseCaseRejectsPersistentPermissionVersionMismatch(t *testing.T) {
	repo := &fakeMessageRepository{}
	conversation := localConversation()
	conversation.PermissionVersion = 7
	decision := allowedDecision()
	decision.PermissionVersion = 8
	useCase := NewSendMessageUseCase(
		&fakePolicy{decision: decision},
		&fakeConversation{context: conversation},
		fakeSequencer{},
		repo,
	)

	_, err := useCase.Execute(context.Background(), testCommand())
	if !errors.Is(err, types.ErrDependencyVersion) {
		t.Fatalf("expected dependency version mismatch, got %v", err)
	}
	if repo.calls != 0 {
		t.Fatalf("repository should not be called")
	}
}

func newTestUseCase(repo MessageRepository) *SendMessageUseCase {
	return NewSendMessageUseCase(
		&fakePolicy{decision: allowedDecision()},
		&fakeConversation{context: localConversation()},
		fakeSequencer{},
		repo,
	)
}

func testCommand() types.SendMessageCommand {
	return types.SendMessageCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "user-1",
			DeviceID:  "device-1",
			SessionID: "session-1",
			TraceID:   "trace-1",
			RequestID: "request-1",
		},
		ConversationID: "conv-1",
		ClientMsgID:    "client-1",
		MessageType:    types.MessageTypeText,
		PayloadJSON:    []byte(`{"text":"hello"}`),
	}
}

func allowedDecision() types.PermissionDecision {
	return types.PermissionDecision{
		Allowed:           true,
		PermissionVersion: 7,
		Classification:    "INTERNAL",
	}
}

func localConversation() types.ConversationSendContext {
	return types.ConversationSendContext{
		MemberVersion:       5,
		PermissionVersion:   7,
		ConversationMode:    types.ConversationModeLocalRowLock,
		FanoutMode:          types.FanoutModeWriteFanout,
		FanoutPolicyVersion: 3,
		CurrentSeqShard:     "local",
	}
}

type fakePolicy struct {
	decision  types.PermissionDecision
	decisions []types.PermissionDecision
	err       error
	calls     int
}

func (f *fakePolicy) CheckSendPermission(context.Context, types.SendMessageCommand) (types.PermissionDecision, error) {
	f.calls++
	if f.err != nil {
		return types.PermissionDecision{}, f.err
	}
	if len(f.decisions) > 0 {
		index := f.calls - 1
		if index >= len(f.decisions) {
			index = len(f.decisions) - 1
		}
		return f.decisions[index], nil
	}
	return f.decision, f.err
}

type fakeConversation struct {
	context  types.ConversationSendContext
	contexts []types.ConversationSendContext
	err      error
	calls    int
}

func (f *fakeConversation) GetSendContext(context.Context, types.SendMessageCommand) (types.ConversationSendContext, error) {
	f.calls++
	if f.err != nil {
		return types.ConversationSendContext{}, f.err
	}
	if len(f.contexts) > 0 {
		index := f.calls - 1
		if index >= len(f.contexts) {
			index = len(f.contexts) - 1
		}
		return f.contexts[index], nil
	}
	return f.context, f.err
}

type fakeSequencer struct{}

func (fakeSequencer) AllocateSeqBlock(context.Context, types.SendMessageCommand) (types.SeqBlock, error) {
	return types.SeqBlock{}, nil
}

type fakeMessageRepository struct {
	result domain.AppendMessageResult
	err    error
	calls  int
	input  domain.AppendMessageInput
}

func (f *fakeMessageRepository) AppendMessage(_ context.Context, input domain.AppendMessageInput) (domain.AppendMessageResult, error) {
	f.calls++
	f.input = input
	return f.result, f.err
}

type fakeAdmission struct {
	err error
}

func (f *fakeAdmission) CheckSendMessage(context.Context) error {
	return f.err
}
