package executor

import (
	"context"

	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

const OperationTypeRepairRequest = "REPAIR_REQUEST"

type OperationExecutor interface {
	Execute(context.Context, types.AdminOperation) (types.OperationExecutionResult, error)
}

type RiskRoutingExecutor struct {
	local    OperationExecutor
	workflow OperationExecutor
}

func NewRiskRoutingExecutor(local OperationExecutor, workflow OperationExecutor) RiskRoutingExecutor {
	return RiskRoutingExecutor{local: local, workflow: workflow}
}

func (executor RiskRoutingExecutor) Execute(ctx context.Context, operation types.AdminOperation) (types.OperationExecutionResult, error) {
	if requiresWorkflow(operation) {
		if executor.workflow == nil {
			return types.OperationExecutionResult{}, types.NewFailedPrecondition("admin operation requires workflow execution")
		}
		return executor.workflow.Execute(ctx, operation)
	}
	if executor.local == nil {
		return types.OperationExecutionResult{}, types.NewUnavailable("admin local executor is not configured")
	}
	return executor.local.Execute(ctx, operation)
}

func requiresWorkflow(operation types.AdminOperation) bool {
	return operation.OperationType == OperationTypeRepairRequest || operation.RiskLevel == types.RiskLevelCritical
}
