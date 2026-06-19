package app

import (
	"context"

	"github.com/qsyy0921/IM/services/skill-registry/internal/types"
)

type Repository interface {
	UpsertSkill(ctx context.Context, skill types.SkillDefinition) (types.SkillDefinition, error)
	GetSkill(ctx context.Context, tenantID types.TenantID, skillID string) (types.SkillDefinition, error)
	ListSkills(ctx context.Context, command types.ListSkillsCommand, fetchLimit int) ([]types.SkillDefinition, error)
}
