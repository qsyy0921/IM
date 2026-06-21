package rpc

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	controlplanev1 "github.com/qsyy0921/IM/api/proto/nexusim/controlplane/v1"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
	"google.golang.org/grpc"
)

func TestControlPlaneCompensationExecutorRollsBackConfig(t *testing.T) {
	client := &fakeControlPlaneClient{
		rollbackResponse: &controlplanev1.RollbackConfigVersionResponse{
			Version: &controlplanev1.ConfigVersion{Version: "v1"},
		},
	}
	executor := NewControlPlaneCompensationExecutor(client, time.Second, []ControlPlaneRollbackInstruction{{
		PayloadRefHash: "sha256:payload",
		Environment:    "prod",
		ConfigKind:     "API_GATEWAY_TENANT_QUOTA",
		BundleKey:      "tenant-a",
		TargetVersion:  "v1",
		OperatorRef:    "operator:rollback",
		ReasonRef:      "reason-sha256:rollback",
	}})

	result, err := executor.ExecuteCompensation(context.Background(), compensationForControlPlane())
	if err != nil {
		t.Fatalf("execute compensation: %v", err)
	}
	if result.Status != types.WorkflowCompensationStatusSucceeded ||
		result.DownstreamService != "control-plane-service" ||
		result.DownstreamRequestRef != "config-rollback:prod:API_GATEWAY_TENANT_QUOTA:tenant-a:v1" {
		t.Fatalf("unexpected result: %+v", result)
	}
	request := client.rollbackRequest
	if request.GetEnvironment() != "prod" ||
		request.GetConfigKind() != "API_GATEWAY_TENANT_QUOTA" ||
		request.GetBundleKey() != "tenant-a" ||
		request.GetTargetVersion() != "v1" ||
		request.GetApprovalRef() != "workflow:wf_1" ||
		request.GetIdempotencyKey() != "workflow-compensation:wfc_1" {
		t.Fatalf("unexpected rollback request: %+v", request)
	}
}

func TestControlPlaneCompensationExecutorUsesResolver(t *testing.T) {
	client := &fakeControlPlaneClient{
		rollbackResponse: &controlplanev1.RollbackConfigVersionResponse{
			Version: &controlplanev1.ConfigVersion{Version: "v3"},
		},
	}
	executor := NewControlPlaneCompensationExecutorWithResolver(client, time.Second, fakeInstructionResolver{
		instruction: types.WorkflowCompensationInstruction{
			PayloadRefHash:  "sha256:payload",
			TargetService:   "control-plane-service",
			TargetOperation: "CONFIG_ROLLBACK",
			InstructionType: types.WorkflowCompensationInstructionTypeControlPlaneRollback,
			Environment:     "prod",
			ConfigKind:      "POLICY_RULESET_REF",
			BundleKey:       "tenant-a",
			TargetVersion:   "v3",
			OperatorRef:     "operator:store",
			ReasonRef:       "reason-sha256:store",
			Status:          types.WorkflowCompensationInstructionStatusActive,
		},
		ok: true,
	})

	result, err := executor.ExecuteCompensation(context.Background(), compensationForControlPlane())
	if err != nil {
		t.Fatalf("execute compensation: %v", err)
	}
	if result.Status != types.WorkflowCompensationStatusSucceeded ||
		result.DownstreamRequestRef != "config-rollback:prod:POLICY_RULESET_REF:tenant-a:v3" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if client.rollbackRequest.GetReasonRef() != "reason-sha256:store" ||
		client.rollbackRequest.GetOperatorRef() != "operator:store" {
		t.Fatalf("unexpected resolver-backed rollback request: %+v", client.rollbackRequest)
	}
}

func TestControlPlaneCompensationExecutorFailsClosedWithoutInstruction(t *testing.T) {
	executor := NewControlPlaneCompensationExecutor(&fakeControlPlaneClient{}, time.Second, nil)

	result, err := executor.ExecuteCompensation(context.Background(), compensationForControlPlane())
	if err != nil {
		t.Fatalf("execute compensation: %v", err)
	}
	if result.Status != types.WorkflowCompensationStatusFailed ||
		result.FailureClass != "COMPENSATION_INSTRUCTION_NOT_FOUND" ||
		result.PublicError != "compensation instruction not found" {
		t.Fatalf("unexpected missing instruction result: %+v", result)
	}
}

func TestControlPlaneCompensationExecutorFailsClosedUnsupportedTarget(t *testing.T) {
	compensation := compensationForControlPlane()
	compensation.TargetService = "audit-service"
	executor := NewControlPlaneCompensationExecutor(&fakeControlPlaneClient{}, time.Second, nil)

	result, err := executor.ExecuteCompensation(context.Background(), compensation)
	if err != nil {
		t.Fatalf("execute compensation: %v", err)
	}
	if result.Status != types.WorkflowCompensationStatusFailed ||
		result.FailureClass != "UNSUPPORTED_COMPENSATION_TARGET" ||
		result.DownstreamService != "audit-service" {
		t.Fatalf("unexpected unsupported target result: %+v", result)
	}
}

func TestLoadControlPlaneRollbackInstructionsRejectsIncompleteInstruction(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/instructions.json"
	if err := writeFile(path, `{"instructions":[{"payload_ref_hash":"sha256:payload"}]}`); err != nil {
		t.Fatalf("write instructions: %v", err)
	}
	if _, err := LoadControlPlaneRollbackInstructions(path); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func compensationForControlPlane() types.WorkflowCompensation {
	return types.WorkflowCompensation{
		TenantID:              "tenant-1",
		WorkflowID:            "wf_1",
		CompensationID:        "wfc_1",
		TargetService:         "control-plane-service",
		TargetOperation:       "CONFIG_ROLLBACK",
		TargetRefHash:         "sha256:target",
		PayloadSchemaVersion:  "admin.config_rollback.v1",
		PayloadRefHash:        "sha256:payload",
		CompensationPolicyRef: "admin.compensation.control_plane.v1",
		ReasonRef:             "reason-sha256:compensation",
	}
}

func writeFile(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

type fakeControlPlaneClient struct {
	rollbackRequest  *controlplanev1.RollbackConfigVersionRequest
	rollbackResponse *controlplanev1.RollbackConfigVersionResponse
	err              error
}

type fakeInstructionResolver struct {
	instruction types.WorkflowCompensationInstruction
	ok          bool
	err         error
}

func (resolver fakeInstructionResolver) ResolveControlPlaneRollbackInstruction(
	context.Context,
	types.WorkflowCompensation,
) (types.WorkflowCompensationInstruction, bool, error) {
	return resolver.instruction, resolver.ok, resolver.err
}

func (client *fakeControlPlaneClient) PublishConfigVersion(context.Context, *controlplanev1.PublishConfigVersionRequest, ...grpc.CallOption) (*controlplanev1.PublishConfigVersionResponse, error) {
	return nil, errors.New("PublishConfigVersion should not be called")
}

func (client *fakeControlPlaneClient) GetConfigSnapshot(context.Context, *controlplanev1.GetConfigSnapshotRequest, ...grpc.CallOption) (*controlplanev1.GetConfigSnapshotResponse, error) {
	return nil, errors.New("GetConfigSnapshot should not be called")
}

func (client *fakeControlPlaneClient) AckAppliedConfigVersion(context.Context, *controlplanev1.AckAppliedConfigVersionRequest, ...grpc.CallOption) (*controlplanev1.AckAppliedConfigVersionResponse, error) {
	return nil, errors.New("AckAppliedConfigVersion should not be called")
}

func (client *fakeControlPlaneClient) RollbackConfigVersion(
	_ context.Context,
	request *controlplanev1.RollbackConfigVersionRequest,
	_ ...grpc.CallOption,
) (*controlplanev1.RollbackConfigVersionResponse, error) {
	client.rollbackRequest = request
	if client.err != nil {
		return nil, client.err
	}
	return client.rollbackResponse, nil
}
