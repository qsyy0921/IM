package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

func TestRiskRoutingExecutorDelegatesNonCriticalToLocal(t *testing.T) {
	local := &fakeExecutor{
		result: types.OperationExecutionResult{
			DownstreamService:    "local",
			DownstreamRequestRef: "operation:admop_local",
			Status:               types.OperationStatusSucceeded,
		},
	}
	workflow := &fakeExecutor{}
	router := NewRiskRoutingExecutor(local, workflow)

	result, err := router.Execute(context.Background(), types.AdminOperation{
		OperationID:   "admop_local",
		OperationType: "USER_BAN",
		RiskLevel:     types.RiskLevelHigh,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.DownstreamService != "local" {
		t.Fatalf("unexpected downstream: %+v", result)
	}
	if local.calls != 1 || workflow.calls != 0 {
		t.Fatalf("unexpected call counts local=%d workflow=%d", local.calls, workflow.calls)
	}
}

func TestRiskRoutingExecutorRoutesRepairRequestToWorkflow(t *testing.T) {
	local := &fakeExecutor{}
	workflow := &fakeExecutor{
		result: types.OperationExecutionResult{
			DownstreamService:    "workflow-service",
			DownstreamRequestRef: "workflow:wf_1",
			Status:               types.OperationStatusSucceeded,
		},
	}
	router := NewRiskRoutingExecutor(local, workflow)

	result, err := router.Execute(context.Background(), types.AdminOperation{
		OperationID:   "admop_repair",
		OperationType: OperationTypeRepairRequest,
		RiskLevel:     types.RiskLevelHigh,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.DownstreamService != "workflow-service" {
		t.Fatalf("unexpected downstream: %+v", result)
	}
	if local.calls != 0 || workflow.calls != 1 {
		t.Fatalf("unexpected call counts local=%d workflow=%d", local.calls, workflow.calls)
	}
}

func TestRiskRoutingExecutorRejectsCriticalWhenWorkflowMissing(t *testing.T) {
	router := NewRiskRoutingExecutor(NewNoopExecutor("local"), nil)

	if _, err := router.Execute(context.Background(), types.AdminOperation{
		OperationID:   "admop_critical",
		OperationType: "CONFIG_PUBLISH",
		RiskLevel:     types.RiskLevelCritical,
	}); !errors.Is(err, types.ErrFailedPrecondition) {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

func TestNoopExecutorRejectsWorkflowRequiredOperations(t *testing.T) {
	noop := NewNoopExecutor("local")
	for _, operation := range []types.AdminOperation{
		{OperationID: "admop_repair", OperationType: OperationTypeRepairRequest, RiskLevel: types.RiskLevelHigh},
		{OperationID: "admop_critical", OperationType: "CONFIG_PUBLISH", RiskLevel: types.RiskLevelCritical},
	} {
		if _, err := noop.Execute(context.Background(), operation); !errors.Is(err, types.ErrFailedPrecondition) {
			t.Fatalf("operation %+v expected failed precondition, got %v", operation, err)
		}
	}
}

type fakeExecutor struct {
	result types.OperationExecutionResult
	err    error
	calls  int
}

func (executor *fakeExecutor) Execute(context.Context, types.AdminOperation) (types.OperationExecutionResult, error) {
	executor.calls++
	return executor.result, executor.err
}
