package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	workflowv1 "github.com/qsyy0921/IM/api/proto/nexusim/workflow/v1"
	"google.golang.org/grpc"
)

func TestParseFlagsBuildsListInstructionDefaults(t *testing.T) {
	cfg := parseFlags([]string{
		"-workflow-id", "wf_1",
		"-status", "active",
	})
	if cfg.mode != "list-compensation-instructions" {
		t.Fatalf("mode = %q", cfg.mode)
	}
	if cfg.status != "ACTIVE" {
		t.Fatalf("status = %q", cfg.status)
	}
	if cfg.pageSize != 50 {
		t.Fatalf("page size = %d", cfg.pageSize)
	}
	if cfg.requestID != "workflow-operator-list-compensation-instructions" || cfg.traceID != cfg.requestID {
		t.Fatalf("unexpected request/trace ids: %+v", cfg)
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestValidateRejectsMissingWorkflowID(t *testing.T) {
	cfg := parseFlags([]string{})
	if err := cfg.validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestExecuteListCompensationInstructions(t *testing.T) {
	cfg := parseFlags([]string{
		"-tenant-id", "tenant-wf",
		"-workflow-id", "wf_1",
		"-status", "ACTIVE",
		"-page-size", "5",
	})
	client := &fakeWorkflowClient{response: &workflowv1.ListWorkflowCompensationInstructionsResponse{
		Instructions: []*workflowv1.WorkflowCompensationInstruction{{
			InstructionId:   "wfi_1",
			WorkflowId:      "wf_1",
			PayloadRefHash:  "sha256:payload",
			TargetService:   "control-plane-service",
			TargetOperation: "CONFIG_ROLLBACK",
			InstructionType: "CONTROL_PLANE_ROLLBACK",
			Environment:     "local",
			ConfigKind:      "quota",
			BundleKey:       "api-gateway.default",
			TargetVersion:   "quota-v1",
			OperatorRef:     "operator:cli",
			ReasonRef:       "reason-sha256:abc",
			Status:          "ACTIVE",
		}},
	}}
	result, err := execute(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if client.request.GetAuthContext().GetTenantId() != "tenant-wf" ||
		client.request.GetWorkflowId() != "wf_1" ||
		client.request.GetStatus() != "ACTIVE" ||
		client.request.GetPageSize() != 5 {
		t.Fatalf("unexpected request: %+v", client.request)
	}
	if len(result.Instructions) != 1 || result.Instructions[0].InstructionID != "wfi_1" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunOutputDoesNotExposePayloadOrReasonBody(t *testing.T) {
	result := commandResult{
		Mode:       "list-compensation-instructions",
		Target:     "127.0.0.1:10750",
		TenantID:   "tenant",
		WorkflowID: "wf_1",
		Instructions: []compensationInstructionRef{{
			InstructionID:  "wfi_1",
			PayloadRefHash: "sha256:payload",
			ReasonRef:      "reason-sha256:abc",
		}},
	}
	var builder strings.Builder
	if err := json.NewEncoder(&builder).Encode(result); err != nil {
		t.Fatalf("encode: %v", err)
	}
	output := builder.String()
	for _, forbidden := range []string{"payload_json", "reason_body", "secret", "token", "password"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output leaked forbidden marker %q: %s", forbidden, output)
		}
	}
}

type fakeWorkflowClient struct {
	workflowv1.WorkflowServiceClient
	request  *workflowv1.ListWorkflowCompensationInstructionsRequest
	response *workflowv1.ListWorkflowCompensationInstructionsResponse
}

func (client *fakeWorkflowClient) ListWorkflowCompensationInstructions(
	_ context.Context,
	request *workflowv1.ListWorkflowCompensationInstructionsRequest,
	_ ...grpc.CallOption,
) (*workflowv1.ListWorkflowCompensationInstructionsResponse, error) {
	client.request = request
	return client.response, nil
}
