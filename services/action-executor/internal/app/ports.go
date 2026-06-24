package app

import (
	"context"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

type SkillCatalogPort interface {
	GetSkill(context.Context, types.AuthContext, string) (types.SkillDefinition, error)
}

type ToolPolicyPort interface {
	CheckToolAction(context.Context, types.CheckToolActionCommand) (types.ToolPolicyDecision, error)
}

type ProposalApprovalPort interface {
	VerifyApprovedProposal(context.Context, types.VerifyApprovedProposalCommand) (types.ApprovedProposal, error)
}

type ToolExecutorPort interface {
	ExecuteTool(context.Context, types.ToolExecutionCommand) (types.ToolExecutionResult, error)
}

type ActionRateLimiterPort interface {
	CheckActionRateLimit(context.Context, types.ActionRateLimitCommand) (types.ActionRateLimitDecision, error)
}

type ExecutionAuditRepository interface {
	RecordExecution(
		context.Context,
		types.ExecutionAudit,
		types.ToolResultProjection,
		*types.ProviderFailureProjection,
	) error
}

type ProviderFailureRedriveRepository interface {
	GetProviderFailureForRedrive(
		context.Context,
		types.TenantID,
		string,
	) (types.ProviderFailureAuditRow, error)
}
