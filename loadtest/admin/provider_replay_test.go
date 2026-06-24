package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	adminv1 "github.com/qsyy0921/IM/api/proto/nexusim/admin/v1"
	"google.golang.org/grpc"
)

func TestProviderReplayHandoffBuildsAdminRequests(t *testing.T) {
	document := validProviderReplayHandoffDocument(t)
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode handoff: %v", err)
	}

	requests, err := providerReplayAdminRequestsFromHandoff(encoded)
	if err != nil {
		t.Fatalf("parse handoff: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one request, got %d", len(requests))
	}
	request := requests[0]
	if request.OperationType != providerReplayOperationType ||
		request.ExpectedWorkflowPolicy != providerReplayApprovalPolicy ||
		request.OperationPayload["redrive_entrypoint"] != providerReplayEntrypoint ||
		request.OperationPayload["direct_execution_allowed"] != false ||
		request.OperationPayload["source_dlq_immutable"] != true {
		t.Fatalf("unexpected request: %+v", request)
	}
}

func TestProviderReplayHandoffRejectsPayloadHashMismatch(t *testing.T) {
	document := validProviderReplayHandoffDocument(t)
	document.AdminOperationRequests[0].OperationPayloadHash = "sha256:wrong"
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode handoff: %v", err)
	}

	if _, err := providerReplayAdminRequestsFromHandoff(encoded); err == nil ||
		!strings.Contains(err.Error(), "payload hash mismatch") {
		t.Fatalf("expected payload hash mismatch, got %v", err)
	}
}

func TestProviderReplayHandoffRejectsSensitiveReasonRef(t *testing.T) {
	document := validProviderReplayHandoffDocument(t)
	document.AdminOperationRequests[0].ReasonRef = "raw:operator-reason"
	document.AdminOperationRequests[0].OperationPayloadHash = providerReplayPayloadHashForTest(t, document.AdminOperationRequests[0].OperationPayload)
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode handoff: %v", err)
	}

	if _, err := providerReplayAdminRequestsFromHandoff(encoded); err == nil ||
		!strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("expected sensitive ref rejection, got %v", err)
	}
}

func TestProviderReplayHandoffRejectsDirectExecutionPayload(t *testing.T) {
	document := validProviderReplayHandoffDocument(t)
	document.AdminOperationRequests[0].OperationPayload["direct_execution_allowed"] = true
	document.AdminOperationRequests[0].OperationPayloadHash = providerReplayPayloadHashForTest(t, document.AdminOperationRequests[0].OperationPayload)
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode handoff: %v", err)
	}

	if _, err := providerReplayAdminRequestsFromHandoff(encoded); err == nil ||
		!strings.Contains(err.Error(), "safety flags") {
		t.Fatalf("expected direct execution rejection, got %v", err)
	}
}

func TestExecuteProviderReplaySubmitCreatesAdminOperation(t *testing.T) {
	document := validProviderReplayHandoffDocument(t)
	path := t.TempDir() + "/handoff.json"
	content, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode handoff: %v", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write handoff: %v", err)
	}
	client := &fakeAdminClient{
		createResponse: &adminv1.CreateAdminOperationResponse{
			Operation: &adminv1.AdminOperation{
				TenantId:             "tenant-provider-replay",
				OperationId:          "admop_provider_replay_1",
				OperationType:        providerReplayOperationType,
				TargetRefHash:        "sha256:provider-failure",
				RiskLevel:            "HIGH",
				PayloadSchemaVersion: providerReplayPayloadSchema,
				PayloadHash:          document.AdminOperationRequests[0].OperationPayloadHash,
				Status:               "SUBMITTED",
			},
		},
	}
	cfg := config{
		mode:                      "provider-replay-submit",
		target:                    "127.0.0.1:10770",
		tenantID:                  "tenant-provider-replay",
		userID:                    "operator-user",
		instanceRef:               "admin-provider-replay-test",
		requestID:                 "admin-provider-replay-submit",
		traceID:                   "trace-provider-replay",
		requestTimeout:            1,
		providerReplayHandoffFile: path,
	}

	result, err := execute(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("execute submit: %v", err)
	}
	if len(result.Operations) != 1 || len(client.createRequests) != 1 {
		t.Fatalf("expected one operation/request: result=%+v requests=%d", result, len(client.createRequests))
	}
	request := client.createRequests[0]
	if request.GetOperationType() != providerReplayOperationType ||
		request.GetPayloadSchemaVersion() != providerReplayPayloadSchema ||
		request.GetAuthContext().GetTenantId() != "tenant-provider-replay" ||
		request.GetOperationPayloadJson() == "" ||
		strings.Contains(request.GetOperationPayloadJson(), "raw") {
		t.Fatalf("unexpected create request: %+v", request)
	}
}

func TestParseFlagsBuildsProviderReplayDefaults(t *testing.T) {
	list := parseFlags([]string{"-mode", "provider-replay-list"})
	if list.operationType != providerReplayOperationType {
		t.Fatalf("provider replay list operation type = %q", list.operationType)
	}
	approve := parseFlags([]string{"-mode", "provider-replay-approve", "-operation-id", "admop_1"})
	if approve.approvalPolicyRef != providerReplayApprovalPolicy ||
		approve.idempotencyKey != "provider-replay-approve:admop_1:operator:cli" {
		t.Fatalf("unexpected provider replay approve defaults: %+v", approve)
	}
}

func validProviderReplayHandoffDocument(t *testing.T) providerReplayHandoffDocument {
	t.Helper()
	payload := map[string]any{
		"provider_failure_ref_hash": "sha256:provider-failure",
		"source_execution_ref_hash": "sha256:execution",
		"source_result_ref_hash":    "sha256:result",
		"replay_candidate_id":       "provider-replay-candidate-1234",
		"redrive_entrypoint":        providerReplayEntrypoint,
		"requires_fresh_proposal":   true,
		"requires_fresh_approval":   true,
		"requires_prepared_audit":   true,
		"requires_new_input":        true,
		"requires_reason_sha256":    true,
		"source_dlq_immutable":      true,
		"direct_execution_allowed":  false,
	}
	return providerReplayHandoffDocument{
		Kind: providerReplayHandoffKind,
		HandoffContract: &providerReplayHandoffContract{
			AdminOperationType:     providerReplayOperationType,
			WorkflowType:           providerReplayWorkflowType,
			TargetService:          providerReplayTargetService,
			TargetOperation:        providerReplayOperationType,
			RedriveEntrypoint:      providerReplayEntrypoint,
			ApprovalPolicyRef:      providerReplayApprovalPolicy,
			PayloadSchemaVersion:   providerReplayPayloadSchema,
			DirectExecutionAllowed: false,
			SourceDLQImmutable:     true,
		},
		AdminOperationRequests: []providerReplayAdminRequest{{
			AuthTenantID:           "tenant-provider-replay",
			OperatorRef:            "operator:alice",
			OperatorRole:           "OPERATOR",
			OperationType:          providerReplayOperationType,
			TargetRefHash:          "sha256:provider-failure",
			RiskLevel:              "HIGH",
			PayloadSchemaVersion:   providerReplayPayloadSchema,
			OperationPayload:       payload,
			OperationPayloadHash:   providerReplayPayloadHashForTest(t, payload),
			ReasonRef:              "reason:provider-replay",
			EvidenceRefs:           []string{"evidence:provider-failure"},
			IdempotencyKey:         "provider-replay-admin:provider-replay-candidate-1234",
			CorrelationID:          "corr-provider-replay",
			CausationID:            "provider-failure-1",
			TraceID:                "trace-provider-replay",
			ExpectedWorkflowPolicy: providerReplayApprovalPolicy,
		}},
	}
}

func providerReplayPayloadHashForTest(t *testing.T, payload map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	return "sha256:" + providerReplaySHA256(string(encoded))
}

type fakeAdminClient struct {
	adminv1.AdminServiceClient
	createRequests []*adminv1.CreateAdminOperationRequest
	createResponse *adminv1.CreateAdminOperationResponse
}

func (client *fakeAdminClient) CreateAdminOperation(
	_ context.Context,
	request *adminv1.CreateAdminOperationRequest,
	_ ...grpc.CallOption,
) (*adminv1.CreateAdminOperationResponse, error) {
	client.createRequests = append(client.createRequests, request)
	return client.createResponse, nil
}
