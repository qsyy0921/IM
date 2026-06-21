package rpc

import (
	"context"
	"errors"
	"testing"
	"time"

	workflowv1 "github.com/qsyy0921/IM/api/proto/nexusim/workflow/v1"
	"github.com/qsyy0921/IM/services/admin-service/internal/infrastructure/executor"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestWorkflowExecutorCreatesRepairApprovalWorkflow(t *testing.T) {
	client := &fakeWorkflowClient{
		response: &workflowv1.CreateWorkflowResponse{
			Workflow: &workflowv1.Workflow{WorkflowId: "wf_admin_repair_1"},
		},
	}
	workflowExecutor := NewWorkflowExecutor(client, time.Second)

	result, err := workflowExecutor.Execute(context.Background(), types.AdminOperation{
		TenantID:             "tenant-admin-rpc-test",
		OperationID:          "admop_repair_1",
		OperationType:        executor.OperationTypeRepairRequest,
		TargetRefHash:        "sha256:target",
		RiskLevel:            types.RiskLevelCritical,
		PayloadSchemaVersion: "admin.repair.v1",
		PayloadHash:          "sha256:payload",
		ReasonRef:            "reason:repair",
		EvidenceRefs:         []string{"evidence:one"},
		RequestedBy:          "operator:alice",
		CorrelationID:        "corr-1",
		TraceID:              "trace-1",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.DownstreamService != "workflow-service" ||
		result.DownstreamRequestRef != "workflow:wf_admin_repair_1" ||
		result.Status != types.OperationStatusSucceeded {
		t.Fatalf("unexpected result: %+v", result)
	}
	request := client.request
	if request.GetWorkflowType() != "REPAIR_APPROVAL" ||
		request.GetTargetService() != "admin-service" ||
		request.GetTargetOperation() != executor.OperationTypeRepairRequest {
		t.Fatalf("unexpected workflow request: %+v", request)
	}
	if request.GetPayloadRefHash() != "sha256:payload" || request.GetReasonRef() != "reason:repair" {
		t.Fatalf("request should only carry refs and hashes: %+v", request)
	}
	if request.GetIdempotencyKey() != "admin-workflow:admop_repair_1" ||
		request.GetCausationId() != "admop_repair_1" {
		t.Fatalf("unexpected correlation fields: %+v", request)
	}
	if request.GetAuthContext().GetServiceName() != "admin-service" ||
		request.GetAuthContext().GetRequestId() != "admop_repair_1" {
		t.Fatalf("unexpected auth context: %+v", request.GetAuthContext())
	}
}

func TestWorkflowExecutorCreatesAdminOperationWorkflowForCriticalOperation(t *testing.T) {
	client := &fakeWorkflowClient{
		response: &workflowv1.CreateWorkflowResponse{
			Workflow: &workflowv1.Workflow{WorkflowId: "wf_admin_operation_1"},
		},
	}
	workflowExecutor := NewWorkflowExecutor(client, time.Second)

	result, err := workflowExecutor.Execute(context.Background(), types.AdminOperation{
		TenantID:             "tenant-admin-rpc-test",
		OperationID:          "admop_config_1",
		OperationType:        executor.OperationTypeConfigPublish,
		TargetRefHash:        "sha256:target",
		RiskLevel:            types.RiskLevelCritical,
		PayloadSchemaVersion: "admin.config_publish.v1",
		PayloadHash:          "sha256:payload",
		ReasonRef:            "reason:config",
		RequestedBy:          "operator:alice",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.DownstreamRequestRef != "workflow:wf_admin_operation_1" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if client.request.GetWorkflowType() != "ADMIN_OPERATION" ||
		client.request.GetApprovalPolicyRef() != "admin.workflow.config_publish.v1" ||
		client.request.GetTargetService() != "control-plane-service" ||
		client.request.GetTargetOperation() != executor.OperationTypeConfigPublish {
		t.Fatalf("unexpected workflow request: %+v", client.request)
	}
}

func TestWorkflowExecutorUsesDefaultAdminPolicyForUnmappedCriticalOperation(t *testing.T) {
	client := &fakeWorkflowClient{
		response: &workflowv1.CreateWorkflowResponse{
			Workflow: &workflowv1.Workflow{WorkflowId: "wf_admin_operation_default_1"},
		},
	}
	workflowExecutor := NewWorkflowExecutor(client, time.Second)

	_, err := workflowExecutor.Execute(context.Background(), types.AdminOperation{
		TenantID:             "tenant-admin-rpc-test",
		OperationID:          "admop_tenant_disable_1",
		OperationType:        "TENANT_DISABLE",
		TargetRefHash:        "sha256:target",
		RiskLevel:            types.RiskLevelCritical,
		PayloadSchemaVersion: "admin.tenant_disable.v1",
		PayloadHash:          "sha256:payload",
		ReasonRef:            "reason:tenant",
		RequestedBy:          "operator:alice",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if client.request.GetWorkflowType() != "ADMIN_OPERATION" ||
		client.request.GetApprovalPolicyRef() != defaultAdminWorkflowPolicy ||
		client.request.GetTargetService() != "admin-service" ||
		client.request.GetTargetOperation() != "TENANT_DISABLE" {
		t.Fatalf("unexpected workflow request: %+v", client.request)
	}
}

func TestWorkflowExecutorCreatesCompensationWorkflow(t *testing.T) {
	client := &fakeWorkflowClient{
		response: &workflowv1.CreateWorkflowResponse{
			Workflow: &workflowv1.Workflow{WorkflowId: "wf_admin_compensation_1"},
		},
	}
	workflowExecutor := NewWorkflowExecutor(client, time.Second)

	workflowRef, err := workflowExecutor.CreateCompensationWorkflow(context.Background(), types.AdminOperation{
		TenantID:             "tenant-admin-rpc-test",
		OperationID:          "admop_comp_1",
		OperationType:        executor.OperationTypeConfigRollback,
		TargetRefHash:        "sha256:target",
		RiskLevel:            types.RiskLevelHigh,
		PayloadSchemaVersion: "admin.config_rollback.v1",
		PayloadHash:          "sha256:payload",
		ReasonRef:            "reason:original",
		EvidenceRefs:         []string{"evidence:one"},
		RequestedBy:          "operator:creator",
		CorrelationID:        "corr-1",
		TraceID:              "trace-1",
	}, "operator:compensator", "reason-sha256:compensation")
	if err != nil {
		t.Fatalf("create compensation workflow: %v", err)
	}
	if workflowRef != "workflow:wf_admin_compensation_1" {
		t.Fatalf("unexpected workflow ref: %s", workflowRef)
	}
	request := client.request
	if request.GetWorkflowType() != "COMPENSATION_REQUEST" ||
		request.GetRequesterRef() != "operator:compensator" ||
		request.GetTargetService() != "control-plane-service" ||
		request.GetTargetOperation() != executor.OperationTypeConfigRollback ||
		request.GetApprovalPolicyRef() != defaultCompensationPolicy ||
		request.GetCompensationPolicyRef() != "admin.compensation.control_plane.v1" {
		t.Fatalf("unexpected compensation workflow request: %+v", request)
	}
	if request.GetPayloadRefHash() != "sha256:payload" ||
		request.GetReasonRef() != "reason-sha256:compensation" ||
		request.GetIdempotencyKey() != "admin-compensation-workflow:admop_comp_1" {
		t.Fatalf("request should only carry refs and stable idempotency: %+v", request)
	}
}

func TestWorkflowExecutorRejectsUnsupportedNonCriticalOperationType(t *testing.T) {
	workflowExecutor := NewWorkflowExecutor(&fakeWorkflowClient{}, time.Second)

	_, err := workflowExecutor.Execute(context.Background(), types.AdminOperation{
		OperationID:   "admop_config",
		OperationType: "CONFIG_PUBLISH",
		RiskLevel:     types.RiskLevelHigh,
	})
	if !errors.Is(err, types.ErrFailedPrecondition) {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

func TestWorkflowExecutorMapsUnavailable(t *testing.T) {
	workflowExecutor := NewWorkflowExecutor(&fakeWorkflowClient{
		err: status.Error(codes.Unavailable, "provider raw text"),
	}, time.Second)

	_, err := workflowExecutor.Execute(context.Background(), types.AdminOperation{
		OperationID:   "admop_repair_unavailable",
		OperationType: executor.OperationTypeRepairRequest,
		RiskLevel:     types.RiskLevelCritical,
	})
	if !errors.Is(err, types.ErrUnavailable) {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

type fakeWorkflowClient struct {
	request  *workflowv1.CreateWorkflowRequest
	response *workflowv1.CreateWorkflowResponse
	err      error
}

func (client *fakeWorkflowClient) CreateWorkflow(_ context.Context, request *workflowv1.CreateWorkflowRequest, _ ...grpc.CallOption) (*workflowv1.CreateWorkflowResponse, error) {
	client.request = request
	if client.err != nil {
		return nil, client.err
	}
	return client.response, nil
}

func (client *fakeWorkflowClient) RecordWorkflowDecision(context.Context, *workflowv1.RecordWorkflowDecisionRequest, ...grpc.CallOption) (*workflowv1.RecordWorkflowDecisionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeWorkflowClient) GetWorkflow(context.Context, *workflowv1.GetWorkflowRequest, ...grpc.CallOption) (*workflowv1.GetWorkflowResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeWorkflowClient) ListWorkflowCompensationInstructions(context.Context, *workflowv1.ListWorkflowCompensationInstructionsRequest, ...grpc.CallOption) (*workflowv1.ListWorkflowCompensationInstructionsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
