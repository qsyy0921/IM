package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/policy-service/internal/domain"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestCheckMessageActionUseCaseAllowsStaticDecision(t *testing.T) {
	useCase := NewCheckMessageActionUseCase(domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 7,
		Classification:    "CONTACT",
	})
	result, err := useCase.Execute(context.Background(), testPolicyCommand(types.MessageActionSend))
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if !result.Allowed || result.PermissionVersion != 7 || result.Classification != "CONTACT" {
		t.Fatalf("unexpected decision: %+v", result)
	}
}

func TestCheckMessageActionUseCaseStaticDecisionUsesConversationPermissionVersion(t *testing.T) {
	useCase := NewCheckMessageActionUseCase(domain.StaticMessagePolicy{
		Allowed:        true,
		Classification: "CONTACT",
	})
	command := testPolicyCommand(types.MessageActionSend)
	command.ConversationPermissionVersion = 42

	result, err := useCase.Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if !result.Allowed || result.PermissionVersion != 42 || result.Classification != "CONTACT" {
		t.Fatalf("unexpected decision: %+v", result)
	}
}

func TestCheckMessageActionUseCaseDeniesModeratedContentBeforeEvaluator(t *testing.T) {
	auditor := &fakePolicyDecisionAuditor{}
	evaluator := &countingEvaluator{
		decision: types.MessageActionDecision{
			TenantID:          "tenant-1",
			UserID:            "user-1",
			ConversationID:    "conv-1",
			Action:            types.MessageActionSend,
			Allowed:           true,
			PermissionVersion: 7,
			Classification:    "STATIC_ALLOW",
		},
	}
	moderator := &fakeContentModerator{
		handled: true,
		decision: types.MessageActionDecision{
			TenantID:          "tenant-1",
			UserID:            "user-1",
			ConversationID:    "conv-1",
			Action:            types.MessageActionSend,
			Allowed:           false,
			PermissionVersion: 11,
			Classification:    "CONTENT_MODERATION_DENIED",
			Reason:            "content moderation policy denied",
			DecisionSource:    types.PolicyDecisionSourceContentModeration,
		},
	}
	useCase := NewCheckMessageActionUseCase(
		evaluator,
		WithMessageContentModerator(moderator),
		WithPolicyDecisionAuditor(auditor),
	)
	command := testPolicyCommand(types.MessageActionSend)
	command.MessageText = "blocked text"

	result, err := useCase.Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if result.Allowed || result.Classification != "CONTENT_MODERATION_DENIED" || result.PermissionVersion != 11 || result.DecisionSource != types.PolicyDecisionSourceContentModeration {
		t.Fatalf("unexpected moderation decision: %+v", result)
	}
	if evaluator.calls != 0 {
		t.Fatalf("moderated content should not call evaluator, got %d", evaluator.calls)
	}
	if !auditor.called || auditor.decision.Classification != "CONTENT_MODERATION_DENIED" {
		t.Fatalf("expected moderation decision audit, got %+v", auditor.decision)
	}
	if moderator.calls != 1 || moderator.command.MessageText != "blocked text" {
		t.Fatalf("expected moderation command text, got calls=%d command=%+v", moderator.calls, moderator.command)
	}
}

func TestCheckMessageActionUseCaseSkipsModerationForDelete(t *testing.T) {
	evaluator := &countingEvaluator{
		decision: types.MessageActionDecision{
			TenantID:          "tenant-1",
			UserID:            "user-1",
			ConversationID:    "conv-1",
			MessageID:         "msg-1",
			Action:            types.MessageActionDelete,
			Allowed:           true,
			PermissionVersion: 7,
			Classification:    "STATIC_ALLOW",
		},
	}
	moderator := &fakeContentModerator{handled: true}
	useCase := NewCheckMessageActionUseCase(evaluator, WithMessageContentModerator(moderator))
	command := testPolicyCommand(types.MessageActionDelete)
	command.MessageText = "ignored"

	result, err := useCase.Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if !result.Allowed || evaluator.calls != 1 {
		t.Fatalf("expected delete to fall through evaluator: result=%+v calls=%d", result, evaluator.calls)
	}
	if moderator.calls != 0 {
		t.Fatalf("delete should not call moderator, got %d", moderator.calls)
	}
}

func TestCheckMessageActionUseCaseDeniesStaticDecision(t *testing.T) {
	useCase := NewCheckMessageActionUseCase(domain.StaticMessagePolicy{
		Allowed:           false,
		PermissionVersion: 3,
		Classification:    "BLOCKED",
		Reason:            "blocked by contact policy",
	})
	result, err := useCase.Execute(context.Background(), testPolicyCommand(types.MessageActionSend))
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if result.Allowed || result.Reason != "blocked by contact policy" {
		t.Fatalf("unexpected deny decision: %+v", result)
	}
}

func TestCheckMessageActionUseCaseObservesEvaluatorDecisionsOnce(t *testing.T) {
	observer := &fakePolicyDecisionObserver{}
	evaluator := &countingEvaluator{
		decision: types.MessageActionDecision{
			TenantID:          "tenant-1",
			UserID:            "user-1",
			ConversationID:    "conv-1",
			Action:            types.MessageActionSend,
			Allowed:           true,
			PermissionVersion: 7,
			Classification:    "STATIC_ALLOW",
		},
	}
	useCase := NewCheckMessageActionUseCase(evaluator, WithPolicyDecisionObserver(observer))

	result, err := useCase.Execute(context.Background(), testPolicyCommand(types.MessageActionSend))
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if !result.Allowed || evaluator.calls != 1 {
		t.Fatalf("expected evaluator allow once, result=%+v calls=%d", result, evaluator.calls)
	}
	if observer.calls != 1 || observer.action != types.MessageActionSend || !observer.allowed || observer.failed {
		t.Fatalf("unexpected observer state: %+v", observer)
	}

	evaluator.decision.Allowed = false
	evaluator.decision.Action = types.MessageActionDelete
	evaluator.decision.Classification = "STATIC_DENY"
	command := testPolicyCommand(types.MessageActionDelete)
	if _, err := useCase.Execute(context.Background(), command); err != nil {
		t.Fatalf("check deny action: %v", err)
	}
	if evaluator.calls != 2 || observer.calls != 2 || observer.action != types.MessageActionDelete || observer.allowed || observer.failed {
		t.Fatalf("expected one deny metric for second decision, evaluator=%d observer=%+v", evaluator.calls, observer)
	}
}

func TestCheckMessageActionUseCaseRecordsAudit(t *testing.T) {
	auditor := &fakePolicyDecisionAuditor{}
	useCase := NewCheckMessageActionUseCase(domain.StaticMessagePolicy{
		Allowed:           false,
		PermissionVersion: 3,
		Classification:    "BLOCKED",
		Reason:            "blocked by contact policy",
	}, WithPolicyDecisionAuditor(auditor))
	command := testPolicyCommand(types.MessageActionSend)
	command.DirectPeerUserID = "peer-1"

	result, err := useCase.Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if !auditor.called {
		t.Fatal("expected audit call")
	}
	if auditor.command.DirectPeerUserID != "peer-1" ||
		auditor.decision.Allowed ||
		auditor.decision.PermissionVersion != result.PermissionVersion ||
		auditor.decision.Classification != "BLOCKED" {
		t.Fatalf("unexpected audit payload: command=%+v decision=%+v", auditor.command, auditor.decision)
	}
}

func TestCheckMessageActionUseCaseDeniesNonSenderMutationBeforeEvaluator(t *testing.T) {
	auditor := &fakePolicyDecisionAuditor{}
	observer := &fakePolicyDecisionObserver{}
	evaluator := &countingEvaluator{
		decision: types.MessageActionDecision{
			TenantID:          "tenant-1",
			UserID:            "user-2",
			ConversationID:    "conv-1",
			MessageID:         "msg-1",
			Action:            types.MessageActionEdit,
			Allowed:           true,
			PermissionVersion: 7,
			Classification:    "STATIC_ALLOW",
		},
	}
	useCase := NewCheckMessageActionUseCase(evaluator, WithPolicyDecisionAuditor(auditor), WithPolicyDecisionObserver(observer))
	command := testPolicyCommand(types.MessageActionEdit)
	command.AuthContext.UserID = "user-2"
	command.MessageSenderUserID = "user-1"
	command.ConversationPermissionVersion = 7

	result, err := useCase.Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if result.Allowed ||
		result.PermissionVersion != 7 ||
		result.Classification != "MESSAGE_OWNERSHIP_DENIED" ||
		result.Reason != "message ownership policy denied" ||
		result.DecisionSource != types.PolicyDecisionSourceMessageOwnership {
		t.Fatalf("unexpected ownership deny: %+v", result)
	}
	if evaluator.calls != 0 {
		t.Fatalf("ownership deny should not call evaluator, got %d", evaluator.calls)
	}
	if !auditor.called || auditor.decision.Classification != "MESSAGE_OWNERSHIP_DENIED" {
		t.Fatalf("expected ownership deny audit, got %+v", auditor.decision)
	}
	if observer.calls != 1 || observer.action != types.MessageActionEdit || observer.allowed || observer.failed {
		t.Fatalf("expected one ownership deny metric, got %+v", observer)
	}
}

func TestCheckMessageActionUseCaseAllowsNonSenderMutationWithOwnershipOverride(t *testing.T) {
	auditor := &fakePolicyDecisionAuditor{}
	observer := &fakePolicyDecisionObserver{}
	evaluator := &countingEvaluator{
		decision: types.MessageActionDecision{
			TenantID:          "tenant-1",
			UserID:            "user-2",
			ConversationID:    "conv-1",
			MessageID:         "msg-1",
			Action:            types.MessageActionRevoke,
			Allowed:           false,
			PermissionVersion: 7,
			Classification:    "STATIC_SHOULD_NOT_APPEAR",
		},
	}
	checker := &fakeOwnershipOverrideChecker{
		allowed: true,
		decision: types.MessageActionDecision{
			TenantID:          "tenant-1",
			UserID:            "user-2",
			ConversationID:    "conv-1",
			MessageID:         "msg-1",
			Action:            types.MessageActionRevoke,
			Allowed:           true,
			PermissionVersion: 7,
			Classification:    "MESSAGE_OWNERSHIP_ROLE_OVERRIDE",
			OwnershipOverride: true,
			DecisionSource:    types.PolicyDecisionSourceOwnershipOverride,
		},
	}
	useCase := NewCheckMessageActionUseCase(
		evaluator,
		WithMessageOwnershipOverrideChecker(checker),
		WithPolicyDecisionAuditor(auditor),
		WithPolicyDecisionObserver(observer),
	)
	command := testPolicyCommand(types.MessageActionRevoke)
	command.AuthContext.UserID = "user-2"
	command.MessageSenderUserID = "user-1"
	command.ConversationPermissionVersion = 7

	result, err := useCase.Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if !result.Allowed || !result.OwnershipOverride || result.Classification != "MESSAGE_OWNERSHIP_ROLE_OVERRIDE" || result.PermissionVersion != 7 || result.DecisionSource != types.PolicyDecisionSourceOwnershipOverride {
		t.Fatalf("unexpected ownership override decision: %+v", result)
	}
	if evaluator.calls != 0 {
		t.Fatalf("ownership override should not call evaluator, got %d", evaluator.calls)
	}
	if checker.calls != 1 || checker.command.AuthContext.UserID != "user-2" {
		t.Fatalf("expected override checker call, got calls=%d command=%+v", checker.calls, checker.command)
	}
	if !auditor.called || auditor.decision.Classification != "MESSAGE_OWNERSHIP_ROLE_OVERRIDE" {
		t.Fatalf("expected ownership override audit, got %+v", auditor.decision)
	}
	if observer.calls != 1 || observer.action != types.MessageActionRevoke || !observer.allowed || observer.failed {
		t.Fatalf("expected one ownership override allow metric, got %+v", observer)
	}
}

func TestCheckMessageActionUseCaseDeniesNonSenderMutationWhenOwnershipOverrideDoesNotMatch(t *testing.T) {
	checker := &fakeOwnershipOverrideChecker{}
	useCase := NewCheckMessageActionUseCase(
		&countingEvaluator{},
		WithMessageOwnershipOverrideChecker(checker),
	)
	command := testPolicyCommand(types.MessageActionDelete)
	command.AuthContext.UserID = "user-2"
	command.MessageSenderUserID = "user-1"
	command.ConversationPermissionVersion = 7

	result, err := useCase.Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if result.Allowed || result.Classification != "MESSAGE_OWNERSHIP_DENIED" {
		t.Fatalf("expected sender-only deny after no override, got %+v", result)
	}
	if checker.calls != 1 {
		t.Fatalf("expected override checker call, got %d", checker.calls)
	}
}

func TestCheckMessageActionUseCaseOwnershipOverrideFailsClosed(t *testing.T) {
	checker := &fakeOwnershipOverrideChecker{err: types.NewDependencyUnavailable("policy ownership override failed")}
	observer := &fakePolicyDecisionObserver{}
	useCase := NewCheckMessageActionUseCase(
		&countingEvaluator{},
		WithMessageOwnershipOverrideChecker(checker),
		WithPolicyDecisionObserver(observer),
	)
	command := testPolicyCommand(types.MessageActionEdit)
	command.AuthContext.UserID = "user-2"
	command.MessageSenderUserID = "user-1"
	command.ConversationPermissionVersion = 7

	_, err := useCase.Execute(context.Background(), command)
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable, got %v", err)
	}
	if observer.calls != 1 || observer.action != types.MessageActionEdit || observer.allowed || !observer.failed {
		t.Fatalf("expected one ownership override error metric, got %+v", observer)
	}
}

func TestCheckMessageActionUseCaseAllowsSenderMutationToEvaluator(t *testing.T) {
	evaluator := &countingEvaluator{
		decision: types.MessageActionDecision{
			TenantID:          "tenant-1",
			UserID:            "user-1",
			ConversationID:    "conv-1",
			MessageID:         "msg-1",
			Action:            types.MessageActionEdit,
			Allowed:           true,
			PermissionVersion: 7,
			Classification:    "STATIC_ALLOW",
		},
	}
	useCase := NewCheckMessageActionUseCase(evaluator)
	command := testPolicyCommand(types.MessageActionEdit)
	command.MessageSenderUserID = command.AuthContext.UserID
	command.ConversationPermissionVersion = 7

	result, err := useCase.Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if !result.Allowed || evaluator.calls != 1 {
		t.Fatalf("expected sender mutation to fall through evaluator: result=%+v calls=%d", result, evaluator.calls)
	}
}

func TestCheckMessageActionUseCaseOwnershipDenyRequiresPermissionVersion(t *testing.T) {
	useCase := NewCheckMessageActionUseCase(domain.StaticMessagePolicy{Allowed: true})
	command := testPolicyCommand(types.MessageActionDelete)
	command.AuthContext.UserID = "user-2"
	command.MessageSenderUserID = "user-1"

	_, err := useCase.Execute(context.Background(), command)
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable, got %v", err)
	}
}

func TestCheckMessageActionUseCaseFailsClosedOnAuditError(t *testing.T) {
	useCase := NewCheckMessageActionUseCase(domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 7,
		Classification:    "CONTACT",
	}, WithPolicyDecisionAuditor(&fakePolicyDecisionAuditor{
		err: types.NewDependencyUnavailable("policy decision audit failed"),
	}))

	_, err := useCase.Execute(context.Background(), testPolicyCommand(types.MessageActionSend))
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable, got %v", err)
	}
}

func TestCheckMessageActionUseCaseValidatesCommand(t *testing.T) {
	useCase := NewCheckMessageActionUseCase(domain.StaticMessagePolicy{Allowed: true})
	_, err := useCase.Execute(context.Background(), types.CheckMessageActionCommand{})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func testPolicyCommand(action types.MessageAction) types.CheckMessageActionCommand {
	command := types.CheckMessageActionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conv-1",
		Action:         action,
	}
	if action != types.MessageActionSend {
		command.MessageID = "msg-1"
	}
	return command
}

type fakePolicyDecisionAuditor struct {
	called   bool
	command  types.CheckMessageActionCommand
	decision types.MessageActionDecision
	err      error
}

func (f *fakePolicyDecisionAuditor) RecordPolicyDecision(
	_ context.Context,
	command types.CheckMessageActionCommand,
	decision types.MessageActionDecision,
) error {
	f.called = true
	f.command = command
	f.decision = decision
	return f.err
}

type fakePolicyDecisionObserver struct {
	calls     int
	action    types.MessageAction
	allowed   bool
	failed    bool
	latencyMS int64
}

func (f *fakePolicyDecisionObserver) RecordPolicyDecisionMetric(action types.MessageAction, allowed bool, failed bool, latencyMS int64) {
	f.calls++
	f.action = action
	f.allowed = allowed
	f.failed = failed
	f.latencyMS = latencyMS
}

type countingEvaluator struct {
	calls    int
	decision types.MessageActionDecision
	err      error
}

func (f *countingEvaluator) DecideMessageAction(context.Context, types.CheckMessageActionCommand) (types.MessageActionDecision, error) {
	f.calls++
	if f.err != nil {
		return types.MessageActionDecision{}, f.err
	}
	return f.decision, nil
}

type fakeOwnershipOverrideChecker struct {
	calls    int
	command  types.CheckMessageActionCommand
	decision types.MessageActionDecision
	allowed  bool
	err      error
}

type fakeContentModerator struct {
	calls    int
	command  types.CheckMessageActionCommand
	decision types.MessageActionDecision
	handled  bool
	err      error
}

func (f *fakeContentModerator) ModerateMessageContent(
	_ context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, bool, error) {
	f.calls++
	f.command = command
	if f.err != nil {
		return types.MessageActionDecision{}, false, f.err
	}
	return f.decision, f.handled, nil
}

func (f *fakeOwnershipOverrideChecker) DecideMessageOwnershipOverride(
	_ context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, bool, error) {
	f.calls++
	f.command = command
	if f.err != nil {
		return types.MessageActionDecision{}, false, f.err
	}
	return f.decision, f.allowed, nil
}
