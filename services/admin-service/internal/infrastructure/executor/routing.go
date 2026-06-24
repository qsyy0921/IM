package executor

import (
	"context"

	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

const (
	OperationTypeRepairRequest                 = "REPAIR_REQUEST"
	OperationTypeProviderReplayRequest         = "PROVIDER_REPLAY_REQUEST"
	OperationTypeConfigPublish                 = "CONFIG_PUBLISH"
	OperationTypeConfigRollback                = "CONFIG_ROLLBACK"
	OperationTypeTenantQuotaChange             = "TENANT_QUOTA_CHANGE"
	OperationTypePolicyRuleChange              = "POLICY_RULE_CHANGE"
	OperationTypeRebacRelationChange           = "REBAC_RELATION_CHANGE"
	OperationTypeAuditExportRequest            = "AUDIT_EXPORT_REQUEST"
	OperationTypeNotificationSuppressionChange = "NOTIFICATION_SUPPRESSION_CHANGE"
)

type OperationExecutor interface {
	Execute(context.Context, types.AdminOperation) (types.OperationExecutionResult, error)
}

type RiskRoutingExecutor struct {
	local    OperationExecutor
	workflow OperationExecutor
}

type OperationTypeRoutingExecutor struct {
	recovery OperationExecutor
	routes   map[string]OperationExecutor
}

func NewRiskRoutingExecutor(local OperationExecutor, workflow OperationExecutor) RiskRoutingExecutor {
	return RiskRoutingExecutor{local: local, workflow: workflow}
}

func NewOperationTypeRoutingExecutor(recovery OperationExecutor, routes map[string]OperationExecutor) OperationTypeRoutingExecutor {
	copied := make(map[string]OperationExecutor, len(routes))
	for operationType, route := range routes {
		if operationType != "" && route != nil {
			copied[operationType] = route
		}
	}
	return OperationTypeRoutingExecutor{recovery: recovery, routes: copied}
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

func (executor OperationTypeRoutingExecutor) Execute(ctx context.Context, operation types.AdminOperation) (types.OperationExecutionResult, error) {
	if route := executor.routes[operation.OperationType]; route != nil {
		return route.Execute(ctx, operation)
	}
	if executor.recovery == nil {
		return types.OperationExecutionResult{}, types.NewUnavailable("admin local executor is not configured")
	}
	return executor.recovery.Execute(ctx, operation)
}

func requiresWorkflow(operation types.AdminOperation) bool {
	return operation.OperationType == OperationTypeRepairRequest ||
		operation.OperationType == OperationTypeProviderReplayRequest ||
		operation.RiskLevel == types.RiskLevelCritical
}
