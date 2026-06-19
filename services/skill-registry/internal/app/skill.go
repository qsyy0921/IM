package app

import (
	"context"

	"github.com/qsyy0921/IM/services/skill-registry/internal/types"
)

type UpsertSkillUseCase struct {
	repository Repository
}

func NewUpsertSkillUseCase(repository Repository) *UpsertSkillUseCase {
	return &UpsertSkillUseCase{repository: repository}
}

func (useCase *UpsertSkillUseCase) Execute(
	ctx context.Context,
	command types.UpsertSkillCommand,
) (types.SkillDefinition, error) {
	if err := command.Validate(); err != nil {
		return types.SkillDefinition{}, err
	}
	skill := command.Skill.Normalized()
	skill.TenantID = command.AuthContext.TenantID
	return useCase.repository.UpsertSkill(ctx, skill)
}

type GetSkillUseCase struct {
	repository Repository
}

func NewGetSkillUseCase(repository Repository) *GetSkillUseCase {
	return &GetSkillUseCase{repository: repository}
}

func (useCase *GetSkillUseCase) Execute(
	ctx context.Context,
	command types.GetSkillCommand,
) (types.SkillDefinition, error) {
	if err := command.Validate(); err != nil {
		return types.SkillDefinition{}, err
	}
	return useCase.repository.GetSkill(ctx, command.AuthContext.TenantID, command.SkillID)
}

type ListSkillsUseCase struct {
	repository Repository
}

func NewListSkillsUseCase(repository Repository) *ListSkillsUseCase {
	return &ListSkillsUseCase{repository: repository}
}

func (useCase *ListSkillsUseCase) Execute(
	ctx context.Context,
	command types.ListSkillsCommand,
) (types.ListSkillsResult, error) {
	if err := command.Validate(); err != nil {
		return types.ListSkillsResult{}, err
	}
	limit := command.EffectiveLimit()
	items, err := useCase.repository.ListSkills(ctx, command, limit+1)
	if err != nil {
		return types.ListSkillsResult{}, err
	}
	result := types.ListSkillsResult{Skills: items}
	if len(result.Skills) > limit {
		result.NextCursor = result.Skills[limit-1].SkillID
		result.Skills = result.Skills[:limit]
	}
	return result, nil
}
