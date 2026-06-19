package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/skill-registry/internal/types"
)

func TestUpsertSkillUseCaseNormalizesAndStoresTenant(t *testing.T) {
	repository := &fakeRepository{}
	useCase := NewUpsertSkillUseCase(repository)
	result, err := useCase.Execute(context.Background(), types.UpsertSkillCommand{
		AuthContext: validAuth(),
		Skill: types.SkillDefinition{
			SkillID:          " skill-1 ",
			DisplayName:      " Draft Reply ",
			Version:          " v1 ",
			ToolName:         " conversation.reply.draft ",
			AllowedActions:   []int32{types.ToolActionCall},
			InputSchemaJSON:  `{"type":"object"}`,
			OutputSchemaJSON: `{"type":"object"}`,
			RiskLevel:        "LOW",
			RequiresApproval: true,
			Tags:             []string{"agent", "agent", " reply "},
		},
	})
	if err != nil {
		t.Fatalf("upsert skill: %v", err)
	}
	if result.TenantID != "tenant-1" ||
		result.SkillID != "skill-1" ||
		result.DisplayName != "Draft Reply" ||
		len(result.Tags) != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if repository.upserted.TenantID != "tenant-1" {
		t.Fatalf("expected usecase to force tenant from auth, got %+v", repository.upserted)
	}
}

func TestUpsertSkillUseCaseRejectsInvalidSchema(t *testing.T) {
	_, err := NewUpsertSkillUseCase(&fakeRepository{}).Execute(context.Background(), types.UpsertSkillCommand{
		AuthContext: validAuth(),
		Skill: types.SkillDefinition{
			SkillID:         "skill-1",
			DisplayName:     "Skill",
			Version:         "v1",
			ToolName:        "tool",
			AllowedActions:  []int32{types.ToolActionCall},
			InputSchemaJSON: `[]`,
			RiskLevel:       "LOW",
		},
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestListSkillsUseCaseReturnsCursor(t *testing.T) {
	repository := &fakeRepository{list: []types.SkillDefinition{
		{SkillID: "a"},
		{SkillID: "b"},
		{SkillID: "c"},
	}}
	result, err := NewListSkillsUseCase(repository).Execute(context.Background(), types.ListSkillsCommand{
		AuthContext: validAuth(),
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if len(result.Skills) != 2 || result.NextCursor != "b" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if repository.fetchLimit != 3 {
		t.Fatalf("expected fetch limit 3, got %d", repository.fetchLimit)
	}
}

func validAuth() types.AuthContext {
	return types.AuthContext{
		TenantID: "tenant-1",
		UserID:   "user-1",
		DeviceID: "device-1",
	}
}

type fakeRepository struct {
	upserted   types.SkillDefinition
	list       []types.SkillDefinition
	fetchLimit int
}

func (repository *fakeRepository) UpsertSkill(
	_ context.Context,
	skill types.SkillDefinition,
) (types.SkillDefinition, error) {
	repository.upserted = skill
	return skill, nil
}

func (repository *fakeRepository) GetSkill(
	_ context.Context,
	tenantID types.TenantID,
	skillID string,
) (types.SkillDefinition, error) {
	return types.SkillDefinition{TenantID: tenantID, SkillID: skillID}, nil
}

func (repository *fakeRepository) ListSkills(
	_ context.Context,
	_ types.ListSkillsCommand,
	fetchLimit int,
) ([]types.SkillDefinition, error) {
	repository.fetchLimit = fetchLimit
	return repository.list, nil
}
