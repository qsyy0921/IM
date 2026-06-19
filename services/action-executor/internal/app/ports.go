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

type ExecutionAuditRepository interface {
	InsertExecutionAudit(context.Context, types.ExecutionAudit) error
}
