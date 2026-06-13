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
	useCase := NewCheckMessageActionUseCase(evaluator, WithPolicyDecisionAuditor(auditor))
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
		result.Reason != "message ownership policy denied" {
		t.Fatalf("unexpected ownership deny: %+v", result)
	}
	if evaluator.calls != 0 {
		t.Fatalf("ownership deny should not call evaluator, got %d", evaluator.calls)
	}
	if !auditor.called || auditor.decision.Classification != "MESSAGE_OWNERSHIP_DENIED" {
		t.Fatalf("expected ownership deny audit, got %+v", auditor.decision)
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
