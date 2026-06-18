package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/policy-service/internal/domain"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestCheckToolActionUseCaseDeniesStaticDefault(t *testing.T) {
	useCase := NewCheckToolActionUseCase(domain.StaticToolPolicy{
		Allowed:           false,
		PermissionVersion: 7,
		Classification:    "TOOL_STATIC_DENY",
	})
	result, err := useCase.Execute(context.Background(), testToolPolicyCommand())
	if err != nil {
		t.Fatalf("check tool action: %v", err)
	}
	if result.Allowed || result.PermissionVersion != 7 || result.Classification != "TOOL_STATIC_DENY" || result.DecisionSource != types.PolicyDecisionSourceFallback {
		t.Fatalf("unexpected decision: %+v", result)
	}
}

func TestCheckToolActionUseCaseRecordsAudit(t *testing.T) {
	auditor := &fakeToolDecisionAuditor{}
	useCase := NewCheckToolActionUseCase(domain.StaticToolPolicy{
		Allowed:           true,
		RequiresApproval:  true,
		PermissionVersion: 9,
		Classification:    "TOOL_APPROVAL_REQUIRED",
		Reason:            "operator approval required",
	}, WithToolDecisionAuditor(auditor))

	result, err := useCase.Execute(context.Background(), testToolPolicyCommand())
	if err != nil {
		t.Fatalf("check tool action: %v", err)
	}
	if !result.Allowed || !result.RequiresApproval {
		t.Fatalf("unexpected decision: %+v", result)
	}
	if !auditor.called || auditor.command.ToolName != "conversation.owner_transfer" || auditor.decision.Classification != "TOOL_APPROVAL_REQUIRED" {
		t.Fatalf("unexpected audit payload: %+v %+v", auditor.command, auditor.decision)
	}
}

func TestCheckToolActionUseCaseFailsClosedOnAuditError(t *testing.T) {
	useCase := NewCheckToolActionUseCase(domain.StaticToolPolicy{
		Allowed:           true,
		PermissionVersion: 9,
		Classification:    "TOOL_ALLOW",
	}, WithToolDecisionAuditor(&fakeToolDecisionAuditor{
		err: types.NewDependencyUnavailable("tool audit unavailable"),
	}))
	_, err := useCase.Execute(context.Background(), testToolPolicyCommand())
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable, got %v", err)
	}
}

func TestCheckToolActionUseCaseValidatesCommand(t *testing.T) {
	useCase := NewCheckToolActionUseCase(domain.StaticToolPolicy{})
	_, err := useCase.Execute(context.Background(), types.CheckToolActionCommand{})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func testToolPolicyCommand() types.CheckToolActionCommand {
	return types.CheckToolActionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ToolName:     "conversation.owner_transfer",
		Action:       types.ToolActionExecute,
		ResourceType: "conversation",
		ResourceID:   "conv-1",
		RiskLevel:    types.ToolRiskLevelHigh,
		Intent:       "transfer owner",
	}
}

type fakeToolDecisionAuditor struct {
	called   bool
	command  types.CheckToolActionCommand
	decision types.ToolActionDecision
	err      error
}

func (f *fakeToolDecisionAuditor) RecordToolDecision(
	_ context.Context,
	command types.CheckToolActionCommand,
	decision types.ToolActionDecision,
) error {
	f.called = true
	f.command = command
	f.decision = decision
	return f.err
}
