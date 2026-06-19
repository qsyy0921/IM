package app

import (
	"context"

	"github.com/qsyy0921/IM/services/mcp-gateway/internal/types"
)

type SkillCatalogPort interface {
	GetSkill(context.Context, types.AuthContext, string) (types.SkillDefinition, error)
}

type ToolPolicyPort interface {
	CheckToolAction(context.Context, types.CheckToolActionCommand) (types.ToolPolicyDecision, error)
}

type AuditRepository interface {
	InsertToolCallAudit(context.Context, types.ToolCallAudit) error
}
