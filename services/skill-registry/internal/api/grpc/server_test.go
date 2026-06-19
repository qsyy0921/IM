package grpc

import (
	"context"
	"testing"

	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	skillregistryv1 "github.com/qsyy0921/IM/api/proto/nexusim/skillregistry/v1"
	"github.com/qsyy0921/IM/services/skill-registry/internal/types"
)

func TestServerUpsertGetAndListSkill(t *testing.T) {
	skill := types.SkillDefinition{
		TenantID:         "tenant-1",
		SkillID:          "skill-1",
		DisplayName:      "Draft Reply",
		Version:          "v1",
		Status:           types.SkillStatusActive,
		ToolName:         "conversation.reply.draft",
		AllowedActions:   []int32{types.ToolActionCall},
		InputSchemaJSON:  `{"type":"object"}`,
		OutputSchemaJSON: `{"type":"object"}`,
		RiskLevel:        "LOW",
		RequiresApproval: true,
	}
	upsertExecutor := &capturingUpsertExecutor{skill: skill}
	getExecutor := &capturingGetExecutor{skill: skill}
	listExecutor := &capturingListExecutor{skill: skill}
	server := NewServer(upsertExecutor, getExecutor, listExecutor)

	upsertResponse, err := server.UpsertSkill(context.Background(), &skillregistryv1.UpsertSkillRequest{
		AuthContext: validProtoAuth(),
		Skill: &skillregistryv1.SkillDefinition{
			SkillId:        "skill-1",
			DisplayName:    "Draft Reply",
			Version:        "v1",
			Status:         skillregistryv1.SkillStatus_SKILL_STATUS_ACTIVE,
			ToolName:       "conversation.reply.draft",
			AllowedActions: []policyv1.ToolAction{policyv1.ToolAction_TOOL_ACTION_CALL},
			RiskLevel:      "LOW",
		},
	})
	if err != nil {
		t.Fatalf("upsert skill: %v", err)
	}
	if upsertResponse.GetSkill().GetAllowedActions()[0] != policyv1.ToolAction_TOOL_ACTION_CALL ||
		upsertExecutor.command.AuthContext.TenantID != "tenant-1" {
		t.Fatalf("unexpected upsert response or command: %+v %+v", upsertResponse, upsertExecutor.command)
	}

	getResponse, err := server.GetSkill(context.Background(), &skillregistryv1.GetSkillRequest{
		AuthContext: validProtoAuth(),
		SkillId:     "skill-1",
	})
	if err != nil {
		t.Fatalf("get skill: %v", err)
	}
	if getResponse.GetSkill().GetSkillId() != "skill-1" || getExecutor.command.SkillID != "skill-1" {
		t.Fatalf("unexpected get response or command: %+v %+v", getResponse, getExecutor.command)
	}

	listResponse, err := server.ListSkills(context.Background(), &skillregistryv1.ListSkillsRequest{
		AuthContext: validProtoAuth(),
		Status:      skillregistryv1.SkillStatus_SKILL_STATUS_ACTIVE,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if len(listResponse.GetSkills()) != 1 || listExecutor.command.Status != types.SkillStatusActive {
		t.Fatalf("unexpected list response or command: %+v %+v", listResponse, listExecutor.command)
	}
}

func TestServerUpsertSkillRequiresAuth(t *testing.T) {
	server := NewServer(&capturingUpsertExecutor{}, &capturingGetExecutor{}, &capturingListExecutor{})
	if _, err := server.UpsertSkill(context.Background(), &skillregistryv1.UpsertSkillRequest{
		Skill: &skillregistryv1.SkillDefinition{},
	}); err == nil {
		t.Fatal("expected auth error")
	}
}

func validProtoAuth() *skillregistryv1.AuthContext {
	return &skillregistryv1.AuthContext{
		TenantId: "tenant-1",
		UserId:   "user-1",
		DeviceId: "device-1",
	}
}

type capturingUpsertExecutor struct {
	command types.UpsertSkillCommand
	skill   types.SkillDefinition
}

func (executor *capturingUpsertExecutor) Execute(
	_ context.Context,
	command types.UpsertSkillCommand,
) (types.SkillDefinition, error) {
	executor.command = command
	return executor.skill, nil
}

type capturingGetExecutor struct {
	command types.GetSkillCommand
	skill   types.SkillDefinition
}

func (executor *capturingGetExecutor) Execute(
	_ context.Context,
	command types.GetSkillCommand,
) (types.SkillDefinition, error) {
	executor.command = command
	return executor.skill, nil
}

type capturingListExecutor struct {
	command types.ListSkillsCommand
	skill   types.SkillDefinition
}

func (executor *capturingListExecutor) Execute(
	_ context.Context,
	command types.ListSkillsCommand,
) (types.ListSkillsResult, error) {
	executor.command = command
	return types.ListSkillsResult{Skills: []types.SkillDefinition{executor.skill}}, nil
}
