package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/mcp-gateway/internal/types"
)

func TestPrepareToolCallUseCaseAllowsRegisteredPolicyAllowedTool(t *testing.T) {
	catalog := &fakeCatalog{skill: activeSkill()}
	policy := &fakePolicy{decision: types.ToolPolicyDecision{
		Allowed:           true,
		RequiresApproval:  true,
		PermissionVersion: 7,
		Classification:    "TOOL_ALLOWED",
		Reason:            "allowed",
		DecisionSource:    "policy-test",
	}}
	audit := &fakeAudit{}
	usecase := NewPrepareToolCallUseCase(catalog, policy, audit)

	result, err := usecase.Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("prepare tool call: %v", err)
	}
	if !result.Allowed || !result.RequiresApproval || result.PermissionVersion != 7 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if policy.command.ToolName != "conversation.note.create" || policy.command.Action != types.ToolActionCall {
		t.Fatalf("unexpected policy command: %+v", policy.command)
	}
	if len(audit.records) != 1 || audit.records[0].InputSHA256 == "" ||
		audit.records[0].Status != types.ToolAuditStatusAllowed {
		t.Fatalf("unexpected audit: %+v", audit.records)
	}
}

func TestPrepareToolCallUseCaseBlocksUnregisteredActionBeforePolicy(t *testing.T) {
	catalog := &fakeCatalog{skill: activeSkill()}
	policy := &fakePolicy{err: errors.New("policy should not be called")}
	audit := &fakeAudit{}
	command := validCommand()
	command.Action = types.ToolActionExecute
	usecase := NewPrepareToolCallUseCase(catalog, policy, audit)

	result, err := usecase.Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("prepare tool call: %v", err)
	}
	if result.Allowed || policy.called || len(audit.records) != 1 ||
		audit.records[0].Classification != "ACTION_NOT_ALLOWED" {
		t.Fatalf("expected local block, got result=%+v policy_called=%v audit=%+v", result, policy.called, audit.records)
	}
}

func TestPrepareToolCallUseCaseValidatesJSON(t *testing.T) {
	command := validCommand()
	command.InputJSON = "{bad"
	usecase := NewPrepareToolCallUseCase(&fakeCatalog{}, &fakePolicy{}, &fakeAudit{})

	_, err := usecase.Execute(context.Background(), command)
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func validCommand() types.PrepareToolCallCommand {
	return types.PrepareToolCallCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		SkillID:        "skill-1",
		ToolName:       "conversation.note.create",
		Action:         types.ToolActionCall,
		ResourceType:   "conversation",
		ResourceID:     "conv-1",
		RiskLevel:      "LOW",
		Intent:         "draft note",
		InputJSON:      `{"note":"hello"}`,
		IdempotencyKey: "idem-1",
	}
}

func activeSkill() types.SkillDefinition {
	return types.SkillDefinition{
		TenantID:         "tenant-1",
		SkillID:          "skill-1",
		Status:           types.SkillStatusActive,
		ToolName:         "conversation.note.create",
		AllowedActions:   []string{types.ToolActionCall},
		RiskLevel:        "LOW",
		RequiresApproval: true,
	}
}

type fakeCatalog struct {
	skill types.SkillDefinition
	err   error
}

func (fake *fakeCatalog) GetSkill(
	context.Context,
	types.AuthContext,
	string,
) (types.SkillDefinition, error) {
	return fake.skill, fake.err
}

type fakePolicy struct {
	decision types.ToolPolicyDecision
	err      error
	command  types.CheckToolActionCommand
	called   bool
}

func (fake *fakePolicy) CheckToolAction(
	_ context.Context,
	command types.CheckToolActionCommand,
) (types.ToolPolicyDecision, error) {
	fake.called = true
	fake.command = command
	return fake.decision, fake.err
}

type fakeAudit struct {
	records []types.ToolCallAudit
	err     error
}

func (fake *fakeAudit) InsertToolCallAudit(_ context.Context, audit types.ToolCallAudit) error {
	fake.records = append(fake.records, audit)
	return fake.err
}
