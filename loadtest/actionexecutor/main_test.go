package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	actionexecutorv1 "github.com/qsyy0921/IM/api/proto/nexusim/actionexecutor/v1"
	"google.golang.org/grpc"
)

func TestProviderReplayRedrivePreflightBuildsLowSensitiveRequest(t *testing.T) {
	fixture := newRedriveFixture(t)
	cfg := fixture.config(false)
	var out bytes.Buffer
	client := &fakeActionExecutorClient{}

	if err := run(context.Background(), cfg, &out, client); err != nil {
		t.Fatalf("run preflight: %v", err)
	}
	if len(client.requests) != 0 {
		t.Fatalf("preflight must not call RedriveProviderFailure")
	}
	var result commandResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v\n%s", err, out.String())
	}
	if result.ExecutedRedrive {
		t.Fatalf("preflight must report executed_redrive=false")
	}
	if result.Request.ResourceIDHash != fixture.resourceHash ||
		result.Request.InputSHA256 != fixture.inputSHA ||
		result.Request.ReasonSHA256 != fixture.reasonSHA ||
		result.Request.UserID != "operator-user" ||
		result.Request.DeviceID != "operator-device" {
		t.Fatalf("unexpected request summary: %+v", result.Request)
	}
	raw := out.String()
	for _, forbidden := range []string{
		fixture.resourceID,
		fixture.inputJSON,
		fixture.reason,
		fixture.resourcePath,
		fixture.inputPath,
		fixture.reasonPath,
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("result leaked forbidden content %q in %s", forbidden, raw)
		}
	}
}

func TestProviderReplayRedriveExecuteCallsActionExecutor(t *testing.T) {
	fixture := newRedriveFixture(t)
	cfg := fixture.config(true)
	client := &fakeActionExecutorClient{
		response: &actionexecutorv1.RedriveProviderFailureResponse{
			ProviderFailureId:  "provider-failure-1",
			SourceExecutionId:  "execution-source-1",
			SourceResultId:     "result-source-1",
			RedriveExecutionId: "execution-redrive-1",
			RedriveResultId:    "result-redrive-1",
			ProposalId:         "proposal-fresh-1",
			ApprovalId:         "approval-fresh-1",
			PreparedAuditId:    "audit-fresh-1",
			SkillId:            "skill.demo",
			ToolName:           "nexusim.local.echo",
			ResourceType:       "conversation",
			ResourceId:         fixture.resourceID,
			Executed:           true,
			ResultStatus:       "SUCCEEDED",
			ResultRef:          "action-executor://executions/execution-redrive-1/results/result-redrive-1",
		},
	}
	var out bytes.Buffer

	if err := run(context.Background(), cfg, &out, client); err != nil {
		t.Fatalf("run execute: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected one redrive request, got %d", len(client.requests))
	}
	request := client.requests[0]
	if request.GetProviderFailureId() != "provider-failure-1" ||
		request.GetReasonSha256() != strings.TrimPrefix(fixture.reasonSHA, "sha256:") ||
		request.GetResourceId() != fixture.resourceID ||
		request.GetInputJson() != fixture.inputJSON ||
		request.GetAuthContext().GetTenantId() != "tenant-provider-replay" {
		t.Fatalf("unexpected redrive request: %+v", request)
	}
	var result commandResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.ExecutedRedrive || result.Response == nil || result.Response.ResourceIDHash != fixture.resourceHash {
		t.Fatalf("unexpected execute result: %+v", result)
	}
	if strings.Contains(out.String(), fixture.resourceID) || strings.Contains(out.String(), fixture.inputJSON) {
		t.Fatalf("execute result leaked raw resource id or input: %s", out.String())
	}
}

func TestProviderReplayRedriveRejectsInputHashMismatch(t *testing.T) {
	fixture := newRedriveFixture(t)
	if err := os.WriteFile(fixture.inputPath, []byte(`{"changed":true}`), 0o600); err != nil {
		t.Fatalf("write mismatched input: %v", err)
	}
	_, err := prepareRedrive(fixture.config(false))
	if err == nil || !strings.Contains(err.Error(), "input-json-file sha256") {
		t.Fatalf("expected input hash mismatch, got %v", err)
	}
}

func TestProviderReplayRedriveRejectsDirectExecutionManifest(t *testing.T) {
	fixture := newRedriveFixture(t)
	manifest := fixture.manifest
	manifest.DirectExecution = true
	writeJSON(t, fixture.manifestPath, manifest)

	_, err := prepareRedrive(fixture.config(false))
	if err == nil || !strings.Contains(err.Error(), "must not claim") {
		t.Fatalf("expected direct execution rejection, got %v", err)
	}
}

func TestProviderReplayRedriveRejectsRepoLocalRawFiles(t *testing.T) {
	fixture := newRedriveFixture(t)
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	repoLocal := filepath.Join(repoRoot, "tmp-provider-replay-redrive-input.json")
	if err := os.WriteFile(repoLocal, []byte(fixture.inputJSON), 0o600); err != nil {
		t.Fatalf("write repo local input: %v", err)
	}
	defer os.Remove(repoLocal)
	cfg := fixture.config(false)
	cfg.inputJSONPath = repoLocal
	_, err = prepareRedrive(cfg)
	if err == nil || !strings.Contains(err.Error(), "must be outside the repository") {
		t.Fatalf("expected repo-local raw file rejection, got %v", err)
	}
}

type redriveFixture struct {
	dir          string
	manifest     redriveInvocationManifest
	manifestPath string
	resourcePath string
	inputPath    string
	reasonPath   string
	resourceID   string
	inputJSON    string
	reason       string
	resourceHash string
	inputSHA     string
	reasonSHA    string
}

func newRedriveFixture(t *testing.T) redriveFixture {
	t.Helper()
	dir := t.TempDir()
	resourceID := "conversation-123"
	inputJSON := `{"message":"fresh approved redrive input"}`
	reason := "operator approved provider replay because provider outage recovered"
	resourceHash := "sha256:" + sha256Hex([]byte(resourceID))
	inputSHA := sha256Ref([]byte(inputJSON))
	reasonSHA := sha256Ref([]byte(reason))
	manifest := redriveInvocationManifest{
		SchemaVersion:      "nexusim.action_executor.provider_replay_redrive_invocation.v1",
		ManifestID:         "provider-replay-redrive-invocation-1",
		Entrypoint:         "RedriveProviderFailure",
		RPCFullMethod:      "/nexusim.actionexecutor.v1.ActionExecutorService/RedriveProviderFailure",
		ExecutesRedrive:    false,
		MutatesFailure:     false,
		SourceDLQImmutable: true,
		DirectExecution:    false,
		RequiresExecution:  true,
		ProviderFailureID:  "provider-failure-1",
		ReplayCandidateID:  "provider-replay-candidate-1",
		AdminOperationID:   "admop-provider-replay-1",
		WorkflowID:         "workflow-provider-replay-1",
		WorkflowStepID:     "workflow-step-1",
		ProposalID:         "proposal-fresh-1",
		ApprovalID:         "approval-fresh-1",
		PreparedAuditID:    "audit-fresh-1",
		SkillID:            "skill.demo",
		ToolName:           "nexusim.local.echo",
		Action:             "EXECUTE",
		ResourceType:       "conversation",
		ResourceIDHash:     resourceHash,
		NewInputSHA256:     inputSHA,
		ReasonSHA256:       reasonSHA,
		RequiredChecks: []string{
			"admin_operation_approved",
			"workflow_approval_recorded",
			"fresh_agent_proposal",
			"fresh_agent_approval",
			"fresh_prepared_audit",
			"new_input_sha256_matches_external_file",
			"reason_sha256_matches_external_file",
			"resource_id_hash_matches_operator_supplied_resource",
			"action_executor_redrive_provider_failure_only",
		},
		ForbiddenContents: []string{
			"raw_provider_input",
			"raw_provider_output",
			"raw_new_input",
		},
	}
	manifest.AuthContextContract.TenantID = "tenant-provider-replay"
	manifest.AuthContextContract.TraceID = "trace-provider-replay"
	manifest.RedriveRequestContract.ProviderFailureID = manifest.ProviderFailureID
	manifest.RedriveRequestContract.ReasonSHA256 = manifest.ReasonSHA256
	manifest.RedriveRequestContract.ProposalID = manifest.ProposalID
	manifest.RedriveRequestContract.ApprovalID = manifest.ApprovalID
	manifest.RedriveRequestContract.PreparedAuditID = manifest.PreparedAuditID
	manifest.RedriveRequestContract.SkillID = manifest.SkillID
	manifest.RedriveRequestContract.ToolName = manifest.ToolName
	manifest.RedriveRequestContract.Action = "EXECUTE"
	manifest.RedriveRequestContract.ResourceType = manifest.ResourceType
	manifest.RedriveRequestContract.ResourceIDHash = manifest.ResourceIDHash
	manifest.RedriveRequestContract.RiskLevel = "HIGH"
	manifest.RedriveRequestContract.Intent = "provider failure redrive after approved repair workflow"
	manifest.RedriveRequestContract.NewInputSHA256 = manifest.NewInputSHA256
	manifest.RedriveRequestContract.IdempotencyKey = "provider-replay-redrive:provider-replay-candidate-1:approval-fresh-1"

	manifestPath := filepath.Join(dir, "provider-replay-redrive-invocation.json")
	resourcePath := filepath.Join(dir, "resource-id.txt")
	inputPath := filepath.Join(dir, "input.json")
	reasonPath := filepath.Join(dir, "reason.txt")
	writeJSON(t, manifestPath, manifest)
	if err := os.WriteFile(resourcePath, []byte(resourceID), 0o600); err != nil {
		t.Fatalf("write resource: %v", err)
	}
	if err := os.WriteFile(inputPath, []byte(inputJSON), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if err := os.WriteFile(reasonPath, []byte(reason), 0o600); err != nil {
		t.Fatalf("write reason: %v", err)
	}
	return redriveFixture{
		dir:          dir,
		manifest:     manifest,
		manifestPath: manifestPath,
		resourcePath: resourcePath,
		inputPath:    inputPath,
		reasonPath:   reasonPath,
		resourceID:   resourceID,
		inputJSON:    inputJSON,
		reason:       reason,
		resourceHash: resourceHash,
		inputSHA:     inputSHA,
		reasonSHA:    reasonSHA,
	}
}

func (fixture redriveFixture) config(execute bool) config {
	return config{
		mode:            "provider-replay-redrive",
		target:          "127.0.0.1:10660",
		requestTimeout:  1,
		manifestPath:    fixture.manifestPath,
		resourceIDPath:  fixture.resourcePath,
		inputJSONPath:   fixture.inputPath,
		reasonPath:      fixture.reasonPath,
		operatorUserID:  "operator-user",
		operatorDevice:  "operator-device",
		operatorSession: "operator-session",
		traceID:         "trace-provider-replay",
		requestID:       "request-provider-replay",
		execute:         execute,
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode json: %v", err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

type fakeActionExecutorClient struct {
	requests []*actionexecutorv1.RedriveProviderFailureRequest
	response *actionexecutorv1.RedriveProviderFailureResponse
}

func (client *fakeActionExecutorClient) RedriveProviderFailure(
	_ context.Context,
	request *actionexecutorv1.RedriveProviderFailureRequest,
	_ ...grpc.CallOption,
) (*actionexecutorv1.RedriveProviderFailureResponse, error) {
	client.requests = append(client.requests, request)
	if client.response != nil {
		return client.response, nil
	}
	return &actionexecutorv1.RedriveProviderFailureResponse{
		ProviderFailureId:  request.GetProviderFailureId(),
		RedriveExecutionId: "execution-redrive",
		ProposalId:         request.GetProposalId(),
		ApprovalId:         request.GetApprovalId(),
		PreparedAuditId:    request.GetPreparedAuditId(),
		SkillId:            request.GetSkillId(),
		ToolName:           request.GetToolName(),
		ResourceType:       request.GetResourceType(),
		ResourceId:         request.GetResourceId(),
		Executed:           true,
		ResultStatus:       "SUCCEEDED",
	}, nil
}
