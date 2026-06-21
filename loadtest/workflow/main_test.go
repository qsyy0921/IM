package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workflowv1 "github.com/qsyy0921/IM/api/proto/nexusim/workflow/v1"
	"google.golang.org/grpc"
)

func TestParseFlagsBuildsListInstructionDefaults(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "list-compensation-instructions",
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

func TestParseFlagsBuildsRecordDecisionDefaults(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "record-decision",
		"-workflow-id", "wf_1",
		"-step-id", "wfs_1",
		"-decider-ref", "operator:alice",
		"-decision", "approve",
		"-evidence-refs", " evidence:one, evidence:one, evidence:two ",
	})
	if cfg.mode != "record-decision" {
		t.Fatalf("mode = %q", cfg.mode)
	}
	if cfg.decision != "APPROVE" {
		t.Fatalf("decision = %q", cfg.decision)
	}
	if cfg.idempotencyKey != "decision:wf_1:wfs_1:APPROVE:operator:alice" {
		t.Fatalf("idempotency key = %q", cfg.idempotencyKey)
	}
	if cfg.correlationID != cfg.requestID || cfg.causationID != "wf_1" {
		t.Fatalf("unexpected correlation/causation ids: %+v", cfg)
	}
	wantEvidence := []string{"evidence:one", "evidence:two"}
	if len(cfg.evidenceRefs) != len(wantEvidence) {
		t.Fatalf("evidence refs = %+v", cfg.evidenceRefs)
	}
	for i := range wantEvidence {
		if cfg.evidenceRefs[i] != wantEvidence[i] {
			t.Fatalf("evidence ref %d = %q", i, cfg.evidenceRefs[i])
		}
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestPrepareConfigLoadsDecisionManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decision.json")
	manifest := `{
		"schema_version": "nexusim.workflow.decision_manifest.v1",
		"workflow_id": "wf_manifest",
		"step_id": "wfs_manifest",
		"decision": "approve",
		"decider_ref": "operator:manifest",
		"decision_policy_ref": "policy:external-approval",
		"reason_ref": "reason-sha256:abc",
		"evidence_refs": ["evidence:ticket-1", " evidence:ticket-1 ", "evidence:ticket-2"],
		"idempotency_key": "external-approval:wf_manifest:wfs_manifest",
		"correlation_id": "corr_external",
		"causation_id": "approval_external",
		"trace_id": "trace_external"
	}`
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	cfg := parseFlags([]string{
		"-mode", "record-decision",
		"-decision-manifest", path,
	})
	prepared, err := prepareConfig(cfg)
	if err != nil {
		t.Fatalf("prepare config: %v", err)
	}
	if err := prepared.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if prepared.workflowID != "wf_manifest" ||
		prepared.stepID != "wfs_manifest" ||
		prepared.decision != "APPROVE" ||
		prepared.deciderRef != "operator:manifest" ||
		prepared.idempotencyKey != "external-approval:wf_manifest:wfs_manifest" ||
		prepared.correlationID != "corr_external" ||
		prepared.causationID != "approval_external" ||
		prepared.traceID != "trace_external" {
		t.Fatalf("unexpected prepared config: %+v", prepared)
	}
	if got := strings.Join(prepared.evidenceRefs, ","); got != "evidence:ticket-1,evidence:ticket-2" {
		t.Fatalf("evidence refs = %q", got)
	}
}

func TestValidateRejectsMissingWorkflowID(t *testing.T) {
	cfg := parseFlags([]string{})
	if err := cfg.validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateRejectsInvalidDecision(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "record-decision",
		"-workflow-id", "wf_1",
		"-step-id", "wfs_1",
		"-decision", "maybe",
	})
	if err := cfg.validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateRejectsSensitiveDecisionRefs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "decider",
			args: []string{"-decider-ref", "bearer-token:operator"},
		},
		{
			name: "reason",
			args: []string{"-reason-ref", "raw:operator approved because secret"},
		},
		{
			name: "evidence",
			args: []string{"-evidence-refs", "evidence:ok,postgres://db-host"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{
				"-mode", "record-decision",
				"-workflow-id", "wf_1",
				"-step-id", "wfs_1",
				"-decision", "APPROVE",
			}
			args = append(args, tt.args...)
			cfg := parseFlags(args)
			if err := cfg.validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestExecuteGetWorkflow(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "get",
		"-tenant-id", "tenant-wf",
		"-workflow-id", "wf_1",
	})
	client := &fakeWorkflowClient{getResponse: &workflowv1.GetWorkflowResponse{
		Workflow: &workflowv1.Workflow{
			WorkflowId:           "wf_1",
			WorkflowType:         "COMPENSATION_REQUEST",
			RiskLevel:            "HIGH",
			TargetService:        "control-plane-service",
			TargetOperation:      "CONFIG_ROLLBACK",
			TargetRefHash:        "sha256:target",
			PayloadSchemaVersion: "admin.config_rollback.v1",
			PayloadRefHash:       "sha256:payload",
			Status:               "WAITING_DECISION",
			CurrentStepId:        "wfs_1",
		},
		Decisions: []*workflowv1.WorkflowDecision{{
			DecisionId:   "wfd_1",
			WorkflowId:   "wf_1",
			StepId:       "wfs_1",
			DeciderRef:   "operator:bob",
			DecisionType: "APPROVE",
		}},
	}}
	result, err := execute(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if client.getRequest.GetAuthContext().GetTenantId() != "tenant-wf" ||
		client.getRequest.GetWorkflowId() != "wf_1" {
		t.Fatalf("unexpected request: %+v", client.getRequest)
	}
	if result.Workflow == nil ||
		result.Workflow.WorkflowID != "wf_1" ||
		result.Workflow.PayloadRefHash != "sha256:payload" ||
		len(result.Decisions) != 1 ||
		result.Decisions[0].DecisionID != "wfd_1" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecuteRecordDecision(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "record-decision",
		"-tenant-id", "tenant-wf",
		"-workflow-id", "wf_1",
		"-step-id", "wfs_1",
		"-decider-ref", "operator:alice",
		"-decision", "APPROVE",
		"-decision-policy-ref", "policy:approval",
		"-reason-ref", "reason-sha256:abc",
		"-evidence-refs", "evidence:one",
		"-idempotency-key", "idem_1",
	})
	client := &fakeWorkflowClient{decisionResponse: &workflowv1.RecordWorkflowDecisionResponse{
		Workflow: &workflowv1.Workflow{
			WorkflowId: "wf_1",
			Status:     "APPROVED",
		},
		Decision: &workflowv1.WorkflowDecision{
			DecisionId:        "wfd_1",
			WorkflowId:        "wf_1",
			StepId:            "wfs_1",
			DeciderRef:        "operator:alice",
			DecisionType:      "APPROVE",
			DecisionPolicyRef: "policy:approval",
			ReasonRef:         "reason-sha256:abc",
			EvidenceRefs:      []string{"evidence:one"},
		},
		Replayed: true,
	}}
	result, err := execute(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if client.decisionRequest.GetWorkflowId() != "wf_1" ||
		client.decisionRequest.GetStepId() != "wfs_1" ||
		client.decisionRequest.GetDecisionType() != "APPROVE" ||
		client.decisionRequest.GetIdempotencyKey() != "idem_1" {
		t.Fatalf("unexpected request: %+v", client.decisionRequest)
	}
	if result.Workflow == nil ||
		result.Workflow.Status != "APPROVED" ||
		result.Decision == nil ||
		result.Decision.DecisionID != "wfd_1" ||
		!result.Replayed {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecuteListCompensationInstructions(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "list-compensation-instructions",
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
		Mode:       "get",
		Target:     "127.0.0.1:10750",
		TenantID:   "tenant",
		WorkflowID: "wf_1",
		Workflow: &workflowRef{
			WorkflowID:     "wf_1",
			PayloadRefHash: "sha256:payload",
			ReasonRef:      "reason-sha256:abc",
		},
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
	getRequest       *workflowv1.GetWorkflowRequest
	getResponse      *workflowv1.GetWorkflowResponse
	decisionRequest  *workflowv1.RecordWorkflowDecisionRequest
	decisionResponse *workflowv1.RecordWorkflowDecisionResponse
	request          *workflowv1.ListWorkflowCompensationInstructionsRequest
	response         *workflowv1.ListWorkflowCompensationInstructionsResponse
}

func (client *fakeWorkflowClient) GetWorkflow(
	_ context.Context,
	request *workflowv1.GetWorkflowRequest,
	_ ...grpc.CallOption,
) (*workflowv1.GetWorkflowResponse, error) {
	client.getRequest = request
	return client.getResponse, nil
}

func (client *fakeWorkflowClient) RecordWorkflowDecision(
	_ context.Context,
	request *workflowv1.RecordWorkflowDecisionRequest,
	_ ...grpc.CallOption,
) (*workflowv1.RecordWorkflowDecisionResponse, error) {
	client.decisionRequest = request
	return client.decisionResponse, nil
}

func (client *fakeWorkflowClient) ListWorkflowCompensationInstructions(
	_ context.Context,
	request *workflowv1.ListWorkflowCompensationInstructionsRequest,
	_ ...grpc.CallOption,
) (*workflowv1.ListWorkflowCompensationInstructionsResponse, error) {
	client.request = request
	return client.response, nil
}
