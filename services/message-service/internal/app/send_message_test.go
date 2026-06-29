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
		&fakeSequencer{},
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

func TestSendMessageUseCasePassesConversationContextToPolicy(t *testing.T) {
	repo := &fakeMessageRepository{
		result: domain.AppendMessageResult{
			MessageID:       "msg-1",
			ConversationSeq: 1,
			AcceptedAt:      time.Unix(100, 0).UTC(),
		},
	}
	conversationContext := localConversation()
	conversationContext.DirectPeerUserID = "user-2"
	policy := &fakePolicy{decision: allowedDecision()}
	useCase := NewSendMessageUseCase(
		policy,
		&fakeConversation{context: conversationContext},
		&fakeSequencer{},
		repo,
	)

	_, err := useCase.Execute(context.Background(), testCommand())
	if err != nil {
		t.Fatalf("execute send message: %v", err)
	}
	if policy.lastConversation.DirectPeerUserID != "user-2" {
		t.Fatalf("policy did not receive direct peer context: %+v", policy.lastConversation)
	}
}

func TestSendMessageUseCaseAdmissionRejectsBeforeDependencyReads(t *testing.T) {
	repo := &fakeMessageRepository{}
	policy := &fakePolicy{decision: allowedDecision()}
	conversation := &fakeConversation{context: localConversation()}
	useCase := NewSendMessageUseCase(
		policy,
		conversation,
		&fakeSequencer{},
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

func TestSendMessageUseCaseReleasesAdmissionPermitOnRepositoryError(t *testing.T) {
	repo := &fakeMessageRepository{err: types.NewDBWriteFailed("test")}
	permit := &fakeAdmissionPermit{}
	useCase := NewSendMessageUseCase(
		&fakePolicy{decision: allowedDecision()},
		&fakeConversation{context: localConversation()},
		&fakeSequencer{},
		repo,
		WithAdmission(&fakeAdmission{permit: permit}),
	)

	_, err := useCase.Execute(context.Background(), testCommand())
	if !errors.Is(err, types.ErrDBWriteFailed) {
		t.Fatalf("expected db write failed, got %v", err)
	}
	if !permit.released {
		t.Fatalf("expected admission permit to be released")
	}
}

func TestSendMessageUseCaseUsesSequencerBlockMode(t *testing.T) {
	repo := &fakeMessageRepository{
		nextConversationSeqFloor: 42,
		result: domain.AppendMessageResult{
			MessageID:       "msg-1",
			ConversationSeq: 42,
			AcceptedAt:      time.Unix(100, 0).UTC(),
		},
	}
	conversation := localConversation()
	conversation.ConversationMode = types.ConversationModeSequencerBlock
	sequencer := &fakeSequencer{block: types.SeqBlock{StartSeq: 42, EndSeq: 42, Epoch: 1}}
	useCase := NewSendMessageUseCase(
		&fakePolicy{decision: allowedDecision()},
		&fakeConversation{context: conversation},
		sequencer,
		repo,
	)

	result, err := useCase.Execute(context.Background(), testCommand())
	if err != nil {
		t.Fatalf("execute sequencer send message: %v", err)
	}
	if result.ConversationSeq != 42 || repo.input.AllocatedSeq != 42 {
		t.Fatalf("expected sequencer seq to be used, result=%+v input=%+v", result, repo.input)
	}
	if sequencer.calls != 1 || sequencer.lastCommand.ClientMsgID != "client-1" {
		t.Fatalf("expected sequencer to be called once, calls=%d command=%+v", sequencer.calls, sequencer.lastCommand)
	}
	if sequencer.lastMinimumStartSeq != 42 {
		t.Fatalf("expected sequencer floor 42, got %d", sequencer.lastMinimumStartSeq)
	}
}

func TestSendMessageUseCaseRejectsInvalidSequencerBlock(t *testing.T) {
	repo := &fakeMessageRepository{}
	conversation := localConversation()
	conversation.ConversationMode = types.ConversationModeSequencerBlock
	useCase := NewSendMessageUseCase(
		&fakePolicy{decision: allowedDecision()},
		&fakeConversation{context: conversation},
		&fakeSequencer{block: types.SeqBlock{StartSeq: 10, EndSeq: 11}},
		repo,
	)

	_, err := useCase.Execute(context.Background(), testCommand())
	if !errors.Is(err, types.ErrSequencerUnavailable) {
		t.Fatalf("expected sequencer unavailable, got %v", err)
	}
	if repo.calls != 0 {
		t.Fatalf("repository should not be called after invalid sequencer block")
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
	useCase := NewSendMessageUseCase(policy, conversation, &fakeSequencer{}, repo)

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

func TestSendMessageUseCaseRetriesConversationDependencyUnavailableOnce(t *testing.T) {
	repo := &fakeMessageRepository{
		result: domain.AppendMessageResult{
			MessageID:       "msg-1",
			ConversationSeq: 1,
			AcceptedAt:      time.Unix(100, 0).UTC(),
		},
	}
	conversation := &fakeConversation{
		errs: []error{
			types.NewDependencyUnavailable("conversation service unavailable"),
			nil,
		},
		context: localConversation(),
	}
	useCase := NewSendMessageUseCase(&fakePolicy{decision: allowedDecision()}, conversation, &fakeSequencer{}, repo)

	_, err := useCase.Execute(context.Background(), testCommand())
	if err != nil {
		t.Fatalf("execute send message: %v", err)
	}
	if conversation.calls != 2 {
		t.Fatalf("expected one conversation retry, got %d calls", conversation.calls)
	}
	if repo.calls != 1 {
		t.Fatalf("repository should be called after retry, got %d", repo.calls)
	}
}

func TestSendMessageUseCaseDoesNotRetryConversationBusinessErrors(t *testing.T) {
	repo := &fakeMessageRepository{}
	conversation := &fakeConversation{err: types.NewConversationNotFound("missing")}
	useCase := NewSendMessageUseCase(&fakePolicy{decision: allowedDecision()}, conversation, &fakeSequencer{}, repo)

	_, err := useCase.Execute(context.Background(), testCommand())
	if !errors.Is(err, types.ErrConversationNotFound) {
		t.Fatalf("expected conversation not found, got %v", err)
	}
	if conversation.calls != 1 {
		t.Fatalf("business error should not be retried, got %d calls", conversation.calls)
	}
	if repo.calls != 0 {
		t.Fatalf("repository should not be called")
	}
}

func TestSendMessageUseCaseStopsAfterConversationDependencyRetry(t *testing.T) {
	repo := &fakeMessageRepository{}
	conversation := &fakeConversation{err: types.NewDependencyUnavailable("conversation service unavailable")}
	useCase := NewSendMessageUseCase(&fakePolicy{decision: allowedDecision()}, conversation, &fakeSequencer{}, repo)

	_, err := useCase.Execute(context.Background(), testCommand())
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable, got %v", err)
	}
	if conversation.calls != 2 {
		t.Fatalf("expected exactly one retry, got %d calls", conversation.calls)
	}
	if repo.calls != 0 {
		t.Fatalf("repository should not be called")
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
		&fakeSequencer{},
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
		&fakeSequencer{},
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
	decision         types.PermissionDecision
	decisions        []types.PermissionDecision
	err              error
	calls            int
	lastConversation types.ConversationSendContext
	lastMessage      types.MessagePolicyContext
}

func (f *fakePolicy) CheckSendPermission(_ context.Context, _ types.SendMessageCommand, conversation types.ConversationSendContext) (types.PermissionDecision, error) {
	f.calls++
	f.lastConversation = conversation
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

func (f *fakePolicy) CheckEditPermission(_ context.Context, _ types.EditMessageCommand, conversation types.ConversationSendContext, message types.MessagePolicyContext) (types.PermissionDecision, error) {
	f.calls++
	f.lastConversation = conversation
	f.lastMessage = message
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
	return f.decision, nil
}

func (f *fakePolicy) CheckRevokePermission(_ context.Context, _ types.RevokeMessageCommand, conversation types.ConversationSendContext, message types.MessagePolicyContext) (types.PermissionDecision, error) {
	f.calls++
	f.lastConversation = conversation
	f.lastMessage = message
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
	return f.decision, nil
}

func (f *fakePolicy) CheckDeletePermission(_ context.Context, _ types.DeleteMessageCommand, conversation types.ConversationSendContext, message types.MessagePolicyContext) (types.PermissionDecision, error) {
	f.calls++
	f.lastConversation = conversation
	f.lastMessage = message
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
	return f.decision, nil
}

type fakeConversation struct {
	context  types.ConversationSendContext
	contexts []types.ConversationSendContext
	err      error
	errs     []error
	calls    int
}

func (f *fakeConversation) GetSendContext(context.Context, types.SendMessageCommand) (types.ConversationSendContext, error) {
	f.calls++
	if len(f.errs) > 0 {
		index := f.calls - 1
		if index >= len(f.errs) {
			index = len(f.errs) - 1
		}
		if f.errs[index] != nil {
			return types.ConversationSendContext{}, f.errs[index]
		}
	}
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

type fakeSequencer struct {
	block               types.SeqBlock
	err                 error
	calls               int
	lastCommand         types.SendMessageCommand
	lastMinimumStartSeq int64
}

func (f *fakeSequencer) AllocateSeqBlock(_ context.Context, command types.SendMessageCommand, minimumStartSeq int64) (types.SeqBlock, error) {
	f.calls++
	f.lastCommand = command
	f.lastMinimumStartSeq = minimumStartSeq
	if f.err != nil {
		return types.SeqBlock{}, f.err
	}
	if f.block.StartSeq == 0 && f.block.EndSeq == 0 {
		return types.SeqBlock{StartSeq: 1, EndSeq: 1, Epoch: 1}, nil
	}
	return f.block, nil
}

type fakeMessageRepository struct {
	result                   domain.AppendMessageResult
	err                      error
	calls                    int
	input                    domain.AppendMessageInput
	nextConversationSeqFloor int64
	nextSeqFloorErr          error
	nextSeqFloorCalls        int

	messagePolicyContext types.MessagePolicyContext
	messagePolicyErr     error
	messagePolicyCalls   int

	revokeResult domain.MessageChangeResult
	revokeErr    error
	revokeCalls  int
	revokeInput  domain.RevokeMessageInput

	editResult domain.MessageChangeResult
	editErr    error
	editCalls  int
	editInput  domain.EditMessageInput

	deleteResult domain.MessageChangeResult
	deleteErr    error
	deleteCalls  int
	deleteInput  domain.DeleteMessageInput
}

func (f *fakeMessageRepository) NextConversationSeqFloor(context.Context, types.TenantID, types.ConversationID) (int64, error) {
	f.nextSeqFloorCalls++
	if f.nextSeqFloorErr != nil {
		return 0, f.nextSeqFloorErr
	}
	if f.nextConversationSeqFloor <= 0 {
		return 1, nil
	}
	return f.nextConversationSeqFloor, nil
}

func (f *fakeMessageRepository) AppendMessage(_ context.Context, input domain.AppendMessageInput) (domain.AppendMessageResult, error) {
	f.calls++
	f.input = input
	return f.result, f.err
}

func (f *fakeMessageRepository) GetMessagePolicyContext(context.Context, types.TenantID, types.ConversationID, types.MessageID) (types.MessagePolicyContext, error) {
	f.messagePolicyCalls++
	if f.messagePolicyErr != nil {
		return types.MessagePolicyContext{}, f.messagePolicyErr
	}
	return f.messagePolicyContext, nil
}

func (f *fakeMessageRepository) RevokeMessage(_ context.Context, input domain.RevokeMessageInput) (domain.MessageChangeResult, error) {
	f.revokeCalls++
	f.revokeInput = input
	return f.revokeResult, f.revokeErr
}

func (f *fakeMessageRepository) EditMessage(_ context.Context, input domain.EditMessageInput) (domain.MessageChangeResult, error) {
	f.editCalls++
	f.editInput = input
	return f.editResult, f.editErr
}

func (f *fakeMessageRepository) DeleteMessage(_ context.Context, input domain.DeleteMessageInput) (domain.MessageChangeResult, error) {
	f.deleteCalls++
	f.deleteInput = input
	return f.deleteResult, f.deleteErr
}

type fakeAdmission struct {
	permit types.AdmissionPermit
	err    error
}

func (f *fakeAdmission) AdmitSendMessage(context.Context) (types.AdmissionPermit, error) {
	return f.permit, f.err
}

type fakeAdmissionPermit struct {
	released bool
}

func (p *fakeAdmissionPermit) Release() {
	p.released = true
}
