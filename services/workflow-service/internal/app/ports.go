package app

import (
	"context"

	"github.com/qsyy0921/IM/services/workflow-service/internal/domain"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

type WorkflowRepository interface {
	CreateWorkflow(context.Context, domain.PreparedWorkflow) (types.Workflow, bool, error)
	RecordWorkflowDecision(context.Context, domain.PreparedDecision) (types.Workflow, types.WorkflowDecision, bool, error)
	GetWorkflow(context.Context, types.GetWorkflowCommand) (types.Workflow, []types.WorkflowDecision, error)
}

type IDGenerator interface {
	NewWorkflowID() string
	NewStepID() string
	NewDecisionID() string
}
