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

func TestParseFlagsBuildsProviderReplayQueueDefaults(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "provider-replay-queue",
		"-tenant-id", "tenant-wf",
	})
	if cfg.mode != "provider-replay-queue" {
		t.Fatalf("mode = %q", cfg.mode)
	}
	if cfg.workflowType != "REPAIR_APPROVAL" ||
		cfg.status != "WAITING_DECISION" ||
		cfg.targetService != "action-executor" ||
		cfg.targetOperation != "PROVIDER_REPLAY_REQUEST" ||
		cfg.approvalPolicyRef != "admin.workflow.provider_replay.v1" {
		t.Fatalf("unexpected provider replay queue defaults: %+v", cfg)
	}
	if cfg.requestID != "workflow-operator-provider-replay-queue" || cfg.traceID != cfg.requestID {
		t.Fatalf("unexpected request/trace ids: %+v", cfg)
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestParseFlagsBuildsOperatorQueuesDefaults(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "operator-queues",
		"-tenant-id", "tenant-wf",
		"-page-size", "3",
	})
	if cfg.mode != "operator-queues" {
		t.Fatalf("mode = %q", cfg.mode)
	}
	if cfg.requestID != "workflow-operator-operator-queues" || cfg.traceID != cfg.requestID {
		t.Fatalf("unexpected request/trace ids: %+v", cfg)
	}
	if cfg.pageSize != 3 {
		t.Fatalf("page size = %d", cfg.pageSize)
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestParseFlagsBuildsExternalCallbackWait(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "external-callback-wait",
		"-tenant-id", "tenant-wf",
		"-workflow-type", "action_approval",
		"-risk-level", "high",
		"-requester-ref", "agent:planner",
		"-requester-service", "agent-service",
		"-target-service", "external-crm",
		"-target-operation", "SYNC_APPROVAL_CALLBACK",
		"-target-ref-hash", "sha256:target",
		"-payload-schema-version", "external.callback_request.v1",
		"-payload-ref-hash", "sha256:payload",
		"-approval-policy-ref", "workflow.external_callback.v1",
		"-timeout-policy-ref", "workflow.external_callback.timeout.v1",
		"-reason-ref", "reason-sha256:abc",
		"-evidence-refs", "evidence:ticket",
		"-idempotency-key", "external-callback:tenant-wf:target",
	})
	if cfg.mode != "external-callback-wait" {
		t.Fatalf("mode = %q", cfg.mode)
	}
	if cfg.workflowType != "ACTION_APPROVAL" ||
		cfg.riskLevel != "HIGH" ||
		cfg.targetService != "external-crm" ||
		cfg.targetOperation != "SYNC_APPROVAL_CALLBACK" ||
		cfg.approvalPolicyRef != "workflow.external_callback.v1" {
		t.Fatalf("unexpected callback wait config: %+v", cfg)
	}
	if cfg.causationID != cfg.requestID {
		t.Fatalf("expected causation id to bind to request id, got %+v", cfg)
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestValidateExternalCallbackWaitRequiresExplicitRefs(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "external-callback-wait",
		"-tenant-id", "tenant-wf",
		"-workflow-type", "ACTION_APPROVAL",
		"-risk-level", "HIGH",
		"-target-service", "external-crm",
		"-target-operation", "SYNC_APPROVAL_CALLBACK",
		"-target-ref-hash", "sha256:target",
		"-payload-schema-version", "external.callback_request.v1",
		"-approval-policy-ref", "workflow.external_callback.v1",
		"-idempotency-key", "external-callback:tenant-wf:target",
	})
	if err := cfg.validate(); err == nil ||
		!strings.Contains(err.Error(), "payload-schema-version and payload-ref-hash are required") {
		t.Fatalf("expected missing payload ref error, got %v", err)
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
		"schema_version": "nexusim.workflow.external_decision_manifest.v1",
		"workflow_id": "wf_manifest",
		"step_id": "wfs_manifest",
		"expected_workflow_type": "repair_approval",
		"expected_status": "waiting_decision",
		"expected_target_service": "action-executor",
		"expected_target_operation": "PROVIDER_REPLAY_REQUEST",
		"expected_target_ref_hash": "sha256:target-binding",
		"expected_payload_schema_version": "admin.provider_replay_request.v1",
		"expected_payload_ref_hash": "sha256:payload-binding",
		"expected_approval_policy_ref": "admin.workflow.provider_replay.v1",
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
		prepared.expectedWorkflowType != "REPAIR_APPROVAL" ||
		prepared.expectedStatus != "WAITING_DECISION" ||
		prepared.expectedTargetService != "action-executor" ||
		prepared.expectedTargetOperation != "PROVIDER_REPLAY_REQUEST" ||
		prepared.expectedTargetRefHash != "sha256:target-binding" ||
		prepared.expectedPayloadSchemaVersion != "admin.provider_replay_request.v1" ||
		prepared.expectedPayloadRefHash != "sha256:payload-binding" ||
		prepared.expectedApprovalPolicyRef != "admin.workflow.provider_replay.v1" ||
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

func TestExecuteExternalCallbackWaitCreatesWorkflowAndManifestTemplate(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "external-callback-wait",
		"-tenant-id", "tenant-wf",
		"-workflow-type", "ACTION_APPROVAL",
		"-risk-level", "HIGH",
		"-requester-ref", "agent:planner",
		"-requester-service", "agent-service",
		"-target-service", "external-crm",
		"-target-operation", "SYNC_APPROVAL_CALLBACK",
		"-target-ref-hash", "sha256:target",
		"-payload-schema-version", "external.callback_request.v1",
		"-payload-ref-hash", "sha256:payload",
		"-approval-policy-ref", "workflow.external_callback.v1",
		"-decision-policy-ref", "workflow.external_callback.decision.v1",
		"-reason-ref", "reason-sha256:abc",
		"-evidence-refs", "evidence:ticket",
		"-idempotency-key", "external-callback:tenant-wf:target",
	})
	client := &fakeWorkflowClient{createResponse: &workflowv1.CreateWorkflowResponse{
		Workflow: &workflowv1.Workflow{
			WorkflowId:           "wf_callback_1",
			WorkflowType:         "ACTION_APPROVAL",
			RiskLevel:            "HIGH",
			RequesterRef:         "agent:planner",
			RequesterService:     "agent-service",
			TargetService:        "external-crm",
			TargetOperation:      "SYNC_APPROVAL_CALLBACK",
			TargetRefHash:        "sha256:target",
			PayloadSchemaVersion: "external.callback_request.v1",
			PayloadRefHash:       "sha256:payload",
			ApprovalPolicyRef:    "workflow.external_callback.v1",
			Status:               "WAITING_DECISION",
			CurrentStepId:        "wfs_callback_1",
			CorrelationId:        "workflow-operator-external-callback-wait",
			TraceId:              "workflow-operator-external-callback-wait",
		},
	}}
	result, err := execute(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if client.createRequest == nil {
		t.Fatal("expected create workflow request")
	}
	if client.createRequest.GetWorkflowType() != "ACTION_APPROVAL" ||
		client.createRequest.GetRiskLevel() != "HIGH" ||
		client.createRequest.GetTargetService() != "external-crm" ||
		client.createRequest.GetTargetOperation() != "SYNC_APPROVAL_CALLBACK" ||
		client.createRequest.GetTargetRefHash() != "sha256:target" ||
		client.createRequest.GetPayloadRefHash() != "sha256:payload" ||
		client.createRequest.GetApprovalPolicyRef() != "workflow.external_callback.v1" {
		t.Fatalf("unexpected create request: %+v", client.createRequest)
	}
	if result.Workflow == nil || result.Workflow.WorkflowID != "wf_callback_1" {
		t.Fatalf("unexpected workflow result: %+v", result.Workflow)
	}
	if result.DecisionManifest == nil ||
		result.DecisionManifest.SchemaVersion != decisionManifestSchemaVersion ||
		result.DecisionManifest.WorkflowID != "wf_callback_1" ||
		result.DecisionManifest.StepID != "wfs_callback_1" ||
		result.DecisionManifest.ExpectedStatus != "WAITING_DECISION" ||
		result.DecisionManifest.ExpectedPayloadRefHash != "sha256:payload" ||
		result.DecisionManifest.Decision != "" ||
		result.DecisionManifest.DeciderRef != "" ||
		result.DecisionManifest.DecisionPolicyRef != "workflow.external_callback.decision.v1" {
		t.Fatalf("unexpected decision manifest template: %+v", result.DecisionManifest)
	}
	if client.decisionRequest != nil {
		t.Fatalf("external callback wait must not record decisions: %+v", client.decisionRequest)
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

func TestExecuteProviderReplayQueue(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "provider-replay-queue",
		"-tenant-id", "tenant-wf",
		"-page-size", "5",
	})
	client := &fakeWorkflowClient{listWorkflowsResponse: &workflowv1.ListWorkflowsResponse{
		Workflows: []*workflowv1.Workflow{{
			WorkflowId:           "wf_provider_replay_1",
			WorkflowType:         "REPAIR_APPROVAL",
			RiskLevel:            "HIGH",
			TargetService:        "action-executor",
			TargetOperation:      "PROVIDER_REPLAY_REQUEST",
			TargetRefHash:        "sha256:provider-failure",
			PayloadSchemaVersion: "admin.provider_replay_request.v1",
			PayloadRefHash:       "sha256:provider-replay-payload",
			ApprovalPolicyRef:    "admin.workflow.provider_replay.v1",
			Status:               "WAITING_DECISION",
			CurrentStepId:        "wfs_provider_replay_1",
		}},
	}}
	result, err := execute(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if client.listWorkflowsRequest.GetAuthContext().GetTenantId() != "tenant-wf" ||
		client.listWorkflowsRequest.GetWorkflowType() != "REPAIR_APPROVAL" ||
		client.listWorkflowsRequest.GetStatus() != "WAITING_DECISION" ||
		client.listWorkflowsRequest.GetTargetService() != "action-executor" ||
		client.listWorkflowsRequest.GetTargetOperation() != "PROVIDER_REPLAY_REQUEST" ||
		client.listWorkflowsRequest.GetApprovalPolicyRef() != "admin.workflow.provider_replay.v1" ||
		client.listWorkflowsRequest.GetPageSize() != 5 {
		t.Fatalf("unexpected request: %+v", client.listWorkflowsRequest)
	}
	if len(result.Workflows) != 1 ||
		result.Workflows[0].WorkflowID != "wf_provider_replay_1" ||
		result.Workflows[0].PayloadRefHash != "sha256:provider-replay-payload" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecuteOperatorQueuesListsLowSensitiveWorkflowQueues(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "operator-queues",
		"-tenant-id", "tenant-wf",
		"-page-size", "2",
	})
	client := &fakeWorkflowClient{listWorkflowsResponses: []*workflowv1.ListWorkflowsResponse{
		{Workflows: []*workflowv1.Workflow{{
			WorkflowId:    "wf_action_1",
			WorkflowType:  "ACTION_APPROVAL",
			RiskLevel:     "HIGH",
			Status:        "WAITING_DECISION",
			CurrentStepId: "wfs_action_1",
		}}},
		{Workflows: []*workflowv1.Workflow{{
			WorkflowId:    "wf_repair_1",
			WorkflowType:  "REPAIR_APPROVAL",
			RiskLevel:     "HIGH",
			Status:        "WAITING_DECISION",
			CurrentStepId: "wfs_repair_1",
		}}},
		{Workflows: []*workflowv1.Workflow{{
			WorkflowId:           "wf_provider_replay_1",
			WorkflowType:         "REPAIR_APPROVAL",
			RiskLevel:            "HIGH",
			TargetService:        "action-executor",
			TargetOperation:      "PROVIDER_REPLAY_REQUEST",
			TargetRefHash:        "sha256:provider-failure",
			PayloadSchemaVersion: "admin.provider_replay_request.v1",
			PayloadRefHash:       "sha256:provider-replay-payload",
			ApprovalPolicyRef:    "admin.workflow.provider_replay.v1",
			Status:               "WAITING_DECISION",
			CurrentStepId:        "wfs_provider_replay_1",
		}}},
		{Workflows: []*workflowv1.Workflow{}},
		{Workflows: []*workflowv1.Workflow{}},
		{Workflows: []*workflowv1.Workflow{}},
	}}
	result, err := execute(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(client.listWorkflowsRequests) != len(defaultOperatorQueues()) {
		t.Fatalf("list requests = %d", len(client.listWorkflowsRequests))
	}
	if client.listWorkflowsRequests[0].GetWorkflowType() != "ACTION_APPROVAL" ||
		client.listWorkflowsRequests[0].GetStatus() != "WAITING_DECISION" ||
		client.listWorkflowsRequests[0].GetPageSize() != 2 {
		t.Fatalf("unexpected action queue request: %+v", client.listWorkflowsRequests[0])
	}
	providerRequest := client.listWorkflowsRequests[2]
	if providerRequest.GetWorkflowType() != "REPAIR_APPROVAL" ||
		providerRequest.GetTargetService() != "action-executor" ||
		providerRequest.GetTargetOperation() != "PROVIDER_REPLAY_REQUEST" ||
		providerRequest.GetApprovalPolicyRef() != "admin.workflow.provider_replay.v1" {
		t.Fatalf("unexpected provider replay queue request: %+v", providerRequest)
	}
	if len(result.OperatorQueues) != len(defaultOperatorQueues()) {
		t.Fatalf("operator queues = %+v", result.OperatorQueues)
	}
	if result.OperatorQueues[0].QueueID != "action-approval" ||
		result.OperatorQueues[0].WorkflowCount != 1 ||
		result.OperatorQueues[2].QueueID != "provider-replay" ||
		result.OperatorQueues[2].WorkflowCount != 1 ||
		result.OperatorQueues[2].Workflows[0].PayloadRefHash != "sha256:provider-replay-payload" {
		t.Fatalf("unexpected operator queue summary: %+v", result.OperatorQueues)
	}
	if client.decisionRequest != nil {
		t.Fatalf("operator queues must not record decisions: %+v", client.decisionRequest)
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

func TestExecuteRecordDecisionWithExternalManifestVerifiesWorkflowBinding(t *testing.T) {
	cfg := externalDecisionConfig()
	client := &fakeWorkflowClient{
		getResponse:      &workflowv1.GetWorkflowResponse{Workflow: matchingExternalDecisionWorkflow()},
		decisionResponse: approvedDecisionResponse(),
	}
	result, err := execute(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if client.getRequest == nil || client.getRequest.GetWorkflowId() != "wf_external" {
		t.Fatalf("expected workflow lookup before decision, got %+v", client.getRequest)
	}
	if client.decisionRequest == nil || client.decisionRequest.GetWorkflowId() != "wf_external" {
		t.Fatalf("expected decision request after binding check, got %+v", client.decisionRequest)
	}
	if result.Workflow == nil || result.Workflow.Status != "APPROVED" || result.Decision == nil {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecuteRecordDecisionWithExternalManifestRejectsBindingMismatch(t *testing.T) {
	cfg := externalDecisionConfig()
	workflow := matchingExternalDecisionWorkflow()
	workflow.PayloadRefHash = "sha256:different-payload"
	client := &fakeWorkflowClient{
		getResponse:      &workflowv1.GetWorkflowResponse{Workflow: workflow},
		decisionResponse: approvedDecisionResponse(),
	}
	if _, err := execute(context.Background(), cfg, client); err == nil ||
		!strings.Contains(err.Error(), "payload_ref_hash binding mismatch") {
		t.Fatalf("expected binding mismatch, got %v", err)
	}
	if client.decisionRequest != nil {
		t.Fatalf("decision should not be recorded after binding mismatch: %+v", client.decisionRequest)
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
		Workflows: []workflowRef{{
			WorkflowID:        "wf_provider_replay_1",
			TargetRefHash:     "sha256:provider-failure",
			PayloadRefHash:    "sha256:provider-replay-payload",
			ApprovalPolicyRef: "admin.workflow.provider_replay.v1",
		}},
		OperatorQueues: []operatorQueueRef{{
			QueueID:       "provider-replay",
			WorkflowType:  "REPAIR_APPROVAL",
			Status:        "WAITING_DECISION",
			WorkflowCount: 1,
			Workflows: []workflowRef{{
				WorkflowID:     "wf_provider_replay_1",
				PayloadRefHash: "sha256:provider-replay-payload",
			}},
		}},
		DecisionManifest: &decisionManifestTemplate{
			SchemaVersion:                decisionManifestSchemaVersion,
			WorkflowID:                   "wf_callback_1",
			StepID:                       "wfs_callback_1",
			ExpectedWorkflowType:         "ACTION_APPROVAL",
			ExpectedStatus:               "WAITING_DECISION",
			ExpectedTargetService:        "external-crm",
			ExpectedTargetOperation:      "SYNC_APPROVAL_CALLBACK",
			ExpectedTargetRefHash:        "sha256:target",
			ExpectedPayloadSchemaVersion: "external.callback_request.v1",
			ExpectedPayloadRefHash:       "sha256:payload",
			ExpectedApprovalPolicyRef:    "workflow.external_callback.v1",
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

func externalDecisionConfig() config {
	cfg := parseFlags([]string{
		"-mode", "record-decision",
		"-tenant-id", "tenant-wf",
		"-workflow-id", "wf_external",
		"-step-id", "wfs_external",
		"-decider-ref", "operator:external",
		"-decision", "APPROVE",
		"-decision-policy-ref", "workflow.external-approval.v1",
		"-reason-ref", "reason-sha256:abc",
		"-evidence-refs", "evidence:ticket",
		"-idempotency-key", "external-approval:wf_external:wfs_external:approve:operator",
	})
	cfg.decisionManifestPath = "manifest.json"
	cfg.expectedWorkflowType = "REPAIR_APPROVAL"
	cfg.expectedStatus = "WAITING_DECISION"
	cfg.expectedTargetService = "action-executor"
	cfg.expectedTargetOperation = "PROVIDER_REPLAY_REQUEST"
	cfg.expectedTargetRefHash = "sha256:target-binding"
	cfg.expectedPayloadSchemaVersion = "admin.provider_replay_request.v1"
	cfg.expectedPayloadRefHash = "sha256:payload-binding"
	cfg.expectedApprovalPolicyRef = "admin.workflow.provider_replay.v1"
	return cfg
}

func matchingExternalDecisionWorkflow() *workflowv1.Workflow {
	return &workflowv1.Workflow{
		WorkflowId:           "wf_external",
		WorkflowType:         "REPAIR_APPROVAL",
		TargetService:        "action-executor",
		TargetOperation:      "PROVIDER_REPLAY_REQUEST",
		TargetRefHash:        "sha256:target-binding",
		PayloadSchemaVersion: "admin.provider_replay_request.v1",
		PayloadRefHash:       "sha256:payload-binding",
		ApprovalPolicyRef:    "admin.workflow.provider_replay.v1",
		Status:               "WAITING_DECISION",
		CurrentStepId:        "wfs_external",
	}
}

func approvedDecisionResponse() *workflowv1.RecordWorkflowDecisionResponse {
	return &workflowv1.RecordWorkflowDecisionResponse{
		Workflow: &workflowv1.Workflow{
			WorkflowId: "wf_external",
			Status:     "APPROVED",
		},
		Decision: &workflowv1.WorkflowDecision{
			DecisionId:        "wfd_external",
			WorkflowId:        "wf_external",
			StepId:            "wfs_external",
			DeciderRef:        "operator:external",
			DecisionType:      "APPROVE",
			DecisionPolicyRef: "workflow.external-approval.v1",
			ReasonRef:         "reason-sha256:abc",
			EvidenceRefs:      []string{"evidence:ticket"},
		},
	}
}

type fakeWorkflowClient struct {
	workflowv1.WorkflowServiceClient
	createRequest          *workflowv1.CreateWorkflowRequest
	createResponse         *workflowv1.CreateWorkflowResponse
	getRequest             *workflowv1.GetWorkflowRequest
	getResponse            *workflowv1.GetWorkflowResponse
	decisionRequest        *workflowv1.RecordWorkflowDecisionRequest
	decisionResponse       *workflowv1.RecordWorkflowDecisionResponse
	listWorkflowsRequest   *workflowv1.ListWorkflowsRequest
	listWorkflowsRequests  []*workflowv1.ListWorkflowsRequest
	listWorkflowsResponse  *workflowv1.ListWorkflowsResponse
	listWorkflowsResponses []*workflowv1.ListWorkflowsResponse
	request                *workflowv1.ListWorkflowCompensationInstructionsRequest
	response               *workflowv1.ListWorkflowCompensationInstructionsResponse
}

func (client *fakeWorkflowClient) CreateWorkflow(
	_ context.Context,
	request *workflowv1.CreateWorkflowRequest,
	_ ...grpc.CallOption,
) (*workflowv1.CreateWorkflowResponse, error) {
	client.createRequest = request
	return client.createResponse, nil
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

func (client *fakeWorkflowClient) ListWorkflows(
	_ context.Context,
	request *workflowv1.ListWorkflowsRequest,
	_ ...grpc.CallOption,
) (*workflowv1.ListWorkflowsResponse, error) {
	client.listWorkflowsRequest = request
	client.listWorkflowsRequests = append(client.listWorkflowsRequests, request)
	if len(client.listWorkflowsResponses) > 0 {
		index := len(client.listWorkflowsRequests) - 1
		if index < len(client.listWorkflowsResponses) {
			return client.listWorkflowsResponses[index], nil
		}
		return &workflowv1.ListWorkflowsResponse{}, nil
	}
	return client.listWorkflowsResponse, nil
}

func (client *fakeWorkflowClient) ListWorkflowCompensationInstructions(
	_ context.Context,
	request *workflowv1.ListWorkflowCompensationInstructionsRequest,
	_ ...grpc.CallOption,
) (*workflowv1.ListWorkflowCompensationInstructionsResponse, error) {
	client.request = request
	return client.response, nil
}
