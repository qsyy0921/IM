package executor

import (
	"context"

	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

type NoopExecutor struct {
	DownstreamService string
}

func NewNoopExecutor(downstreamService string) NoopExecutor {
	if downstreamService == "" {
		downstreamService = "local-noop"
	}
	return NoopExecutor{DownstreamService: downstreamService}
}

func (executor NoopExecutor) Execute(_ context.Context, operation types.AdminOperation) (types.OperationExecutionResult, error) {
	if requiresWorkflow(operation) {
		return types.OperationExecutionResult{}, types.NewFailedPrecondition("admin operation requires workflow execution")
	}
	return types.OperationExecutionResult{
		DownstreamService:    executor.DownstreamService,
		DownstreamRequestRef: "operation:" + operation.OperationID,
		Status:               types.OperationStatusSucceeded,
	}, nil
}
