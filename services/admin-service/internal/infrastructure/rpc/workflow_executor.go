package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	workflowv1 "github.com/qsyy0921/IM/api/proto/nexusim/workflow/v1"
	"github.com/qsyy0921/IM/services/admin-service/internal/infrastructure/executor"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	workflowTypeRepairApproval = "REPAIR_APPROVAL"

	defaultWorkflowApprovalPolicy = "admin.workflow.repair.v1"
	defaultWorkflowTimeoutPolicy  = "admin.workflow.timeout.default.v1"
)

type WorkflowExecutor struct {
	client  workflowv1.WorkflowServiceClient
	timeout time.Duration
}

func NewWorkflowExecutor(client workflowv1.WorkflowServiceClient, timeout time.Duration) WorkflowExecutor {
	if timeout <= 0 {
		timeout = time.Second
	}
	return WorkflowExecutor{client: client, timeout: timeout}
}

func DialWorkflowExecutor(_ context.Context, addr string, timeout time.Duration) (WorkflowExecutor, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return WorkflowExecutor{}, nil, errors.New("workflow-service address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return WorkflowExecutor{}, nil, err
	}
	return NewWorkflowExecutor(workflowv1.NewWorkflowServiceClient(conn), timeout), conn.Close, nil
}

func (executor WorkflowExecutor) Execute(ctx context.Context, operation types.AdminOperation) (types.OperationExecutionResult, error) {
	if executor.client == nil {
		return types.OperationExecutionResult{}, types.NewUnavailable("workflow executor is not configured")
	}
	workflowType, err := workflowTypeForAdminOperation(operation)
	if err != nil {
		return types.OperationExecutionResult{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx, executor.timeout)
	defer cancel()
	response, err := executor.client.CreateWorkflow(callCtx, &workflowv1.CreateWorkflowRequest{
		AuthContext: &workflowv1.AuthContext{
			TenantId:    string(operation.TenantID),
			ServiceName: "admin-service",
			InstanceRef: "operation-worker",
			TraceId:     operation.TraceID,
			RequestId:   operation.OperationID,
		},
		RequesterRef:         operation.RequestedBy,
		RequesterService:     "admin-service",
		WorkflowType:         workflowType,
		RiskLevel:            operation.RiskLevel,
		TargetRefHash:        operation.TargetRefHash,
		TargetService:        "admin-service",
		TargetOperation:      operation.OperationType,
		ApprovalPolicyRef:    defaultWorkflowApprovalPolicy,
		TimeoutPolicyRef:     defaultWorkflowTimeoutPolicy,
		PayloadSchemaVersion: operation.PayloadSchemaVersion,
		PayloadRefHash:       operation.PayloadHash,
		ReasonRef:            operation.ReasonRef,
		EvidenceRefs:         append([]string(nil), operation.EvidenceRefs...),
		IdempotencyKey:       "admin-workflow:" + operation.OperationID,
		CorrelationId:        operation.CorrelationID,
		CausationId:          firstNonEmpty(operation.CausationID, operation.OperationID),
		TraceId:              operation.TraceID,
	})
	if err != nil {
		return types.OperationExecutionResult{}, mapWorkflowError(err)
	}
	workflow := response.GetWorkflow()
	if workflow == nil || strings.TrimSpace(workflow.GetWorkflowId()) == "" {
		return types.OperationExecutionResult{}, types.NewUnavailable("workflow response is incomplete")
	}
	return types.OperationExecutionResult{
		DownstreamService:    "workflow-service",
		DownstreamRequestRef: "workflow:" + workflow.GetWorkflowId(),
		Status:               types.OperationStatusSucceeded,
	}, nil
}

func workflowTypeForAdminOperation(operation types.AdminOperation) (string, error) {
	if operation.OperationType == executor.OperationTypeRepairRequest {
		return workflowTypeRepairApproval, nil
	}
	return "", types.NewFailedPrecondition("admin operation workflow type is unsupported")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func mapWorkflowError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return types.NewUnavailable("workflow temporarily unavailable")
	}
	st, ok := status.FromError(err)
	if !ok {
		return types.NewUnavailable("workflow temporarily unavailable")
	}
	switch st.Code() {
	case codes.InvalidArgument:
		return types.NewInvalidArgument("workflow request invalid")
	case codes.PermissionDenied:
		return types.NewPermissionDenied("workflow permission denied")
	case codes.FailedPrecondition:
		return types.NewFailedPrecondition("workflow precondition failed")
	case codes.AlreadyExists:
		return types.NewAlreadyExists("workflow already exists")
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.NewUnavailable("workflow temporarily unavailable")
	default:
		return types.NewUnavailable("workflow temporarily unavailable")
	}
}
