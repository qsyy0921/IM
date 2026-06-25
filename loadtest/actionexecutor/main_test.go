package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	actionexecutorv1 "github.com/qsyy0921/IM/api/proto/nexusim/actionexecutor/v1"
	auditv1 "github.com/qsyy0921/IM/api/proto/nexusim/audit/v1"
	"google.golang.org/grpc"
)

func TestProviderReplayRedrivePreflightBuildsLowSensitiveRequest(t *testing.T) {
	fixture := newRedriveFixture(t)
	cfg := fixture.config(false)
	var out bytes.Buffer
	client := &fakeActionExecutorClient{}

	if err := run(context.Background(), cfg, &out, operatorClients{actionExecutor: client}); err != nil {
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

	if err := run(context.Background(), cfg, &out, operatorClients{actionExecutor: client}); err != nil {
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

func TestExternalAuditAppendPreflightBuildsLowSensitiveRequest(t *testing.T) {
	fixture := newAuditAppendFixture(t)
	cfg := fixture.config(false)
	var out bytes.Buffer
	client := &fakeAuditClient{}

	if err := run(context.Background(), cfg, &out, operatorClients{audit: client}); err != nil {
		t.Fatalf("run audit preflight: %v", err)
	}
	if len(client.requests) != 0 {
		t.Fatalf("preflight must not call AppendAuditRecord")
	}
	var result auditAppendCommandResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v\n%s", err, out.String())
	}
	if result.ExecutedAppend {
		t.Fatalf("preflight must report executed_append=false")
	}
	if result.Request.SourceService != "action-executor" ||
		result.Request.SourceEventID != "execution-redrive-1" ||
		result.Request.AttributesSHA256 != fixture.attributesSHA ||
		result.Request.UserID != "operator-user" {
		t.Fatalf("unexpected audit request summary: %+v", result.Request)
	}
	raw := out.String()
	for _, forbidden := range []string{
		fixture.rawProviderInput,
		fixture.manifestPath,
		fixture.attributesJSON,
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("audit result leaked forbidden content %q in %s", forbidden, raw)
		}
	}
}

func TestExternalAuditAppendExecuteCallsAuditService(t *testing.T) {
	fixture := newAuditAppendFixture(t)
	cfg := fixture.config(true)
	client := &fakeAuditClient{
		response: &auditv1.AppendAuditRecordResponse{
			Record: &auditv1.AuditRecord{
				AuditId:            "audit-action-redrive-1",
				RecordHash:         "record-hash-1",
				PreviousRecordHash: "previous-hash-1",
				IdempotencyKey:     "action-executor:audit:execution-redrive-1",
			},
		},
	}
	var out bytes.Buffer

	if err := run(context.Background(), cfg, &out, operatorClients{audit: client}); err != nil {
		t.Fatalf("run audit execute: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected one append request, got %d", len(client.requests))
	}
	request := client.requests[0]
	if request.GetSourceService() != "action-executor" ||
		request.GetSourceEventId() != "execution-redrive-1" ||
		request.GetAuthContext().GetTenantId() != "tenant-provider-replay" ||
		request.GetAttributesJson() != fixture.attributesJSON {
		t.Fatalf("unexpected append request: %+v", request)
	}
	var result auditAppendCommandResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.ExecutedAppend || result.Response == nil || result.Response.AuditID != "audit-action-redrive-1" {
		t.Fatalf("unexpected audit execute result: %+v", result)
	}
	if strings.Contains(out.String(), fixture.attributesJSON) {
		t.Fatalf("execute result leaked attributes json: %s", out.String())
	}
}

func TestExternalAuditAppendAcceptsWorkflowCompensationManifest(t *testing.T) {
	fixture := newAuditAppendFixture(t)
	manifest := fixture.manifest
	attributes := `{"operation_id":"wf-comp-1","operation_type":"CONFIG_ROLLBACK","source_ref":"wfc-comp-1","status":"SUCCEEDED","target_ref_hash":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","payload_hash":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","payload_schema_version":"admin.config_rollback.v1","downstream_service":"control-plane-service","downstream_request_ref":"config-rollback:prod:quota:v1"}`
	attributesJSON := compactJSON(json.RawMessage(attributes))
	manifest.SchemaVersion = "nexusim.audit.external_append.v1"
	manifest.ManifestID = "workflow-compensation-audit-append-1"
	manifest.SourceManifestID = "workflow-compensation-execution-result-1"
	manifest.AuditStream = "security"
	manifest.SourceService = "workflow-service"
	manifest.SourceEventID = "wfc-comp-1"
	manifest.RecordType = "WORKFLOW_COMPENSATION_EXECUTION"
	manifest.ActorRef = "service:workflow-service"
	manifest.SubjectRef = "workflow:wf-comp-1"
	manifest.ResourceRef = "workflow:wf-comp-1:compensation:wfc-comp-1"
	manifest.Action = "EXECUTE_COMPENSATION"
	manifest.Outcome = "SUCCEEDED"
	manifest.ReasonCode = "WORKFLOW_COMPENSATION_EXECUTED"
	manifest.RiskLevel = "HIGH"
	manifest.AttributesJSON = json.RawMessage(attributesJSON)
	manifest.AttributesSHA256 = sha256Ref([]byte(attributesJSON))
	manifest.IdempotencyKey = "workflow-service:audit:wfc-comp-1:SUCCEEDED"
	manifest.CorrelationID = "wf-comp-1"
	manifest.CausationID = "workflow-compensation-execution-result-1"
	manifest.RequiredChecks = []string{
		"source_compensation_result_manifest_verified",
		"workflow_compensation_result_low_sensitive",
		"no_raw_compensation_payload",
		"audit_service_append_only",
		"idempotency_key_present",
	}
	writeJSON(t, fixture.manifestPath, manifest)

	cfg := fixture.config(true)
	client := &fakeAuditClient{}
	var out bytes.Buffer
	if err := run(context.Background(), cfg, &out, operatorClients{audit: client}); err != nil {
		t.Fatalf("run workflow compensation audit append: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected one append request, got %d", len(client.requests))
	}
	request := client.requests[0]
	if request.GetSourceService() != "workflow-service" ||
		request.GetRecordType() != "WORKFLOW_COMPENSATION_EXECUTION" ||
		request.GetAction() != "EXECUTE_COMPENSATION" ||
		request.GetAttributesJson() != attributesJSON {
		t.Fatalf("unexpected workflow audit append request: %+v", request)
	}
	if strings.Contains(out.String(), attributesJSON) {
		t.Fatalf("workflow audit append result leaked attributes json: %s", out.String())
	}
}

func TestExternalAuditAppendRejectsSensitiveManifest(t *testing.T) {
	fixture := newAuditAppendFixture(t)
	manifest := fixture.manifest
	manifest.ForbiddenContents = append(manifest.ForbiddenContents, "raw_input")
	manifest.ResourceRef = "resource-without-sensitive-content"
	writeRawJSON(t, fixture.manifestPath, map[string]any{
		"schema_version":              manifest.SchemaVersion,
		"manifest_id":                 manifest.ManifestID,
		"source_service":              manifest.SourceService,
		"source_event_id":             manifest.SourceEventID,
		"record_type":                 manifest.RecordType,
		"resource_ref":                manifest.ResourceRef,
		"action":                      manifest.Action,
		"outcome":                     manifest.Outcome,
		"occurred_at_unix_ms":         manifest.OccurredAtUnixMs,
		"attributes_json":             json.RawMessage(manifest.AttributesJSON),
		"attributes_sha256":           manifest.AttributesSHA256,
		"idempotency_key":             manifest.IdempotencyKey,
		"requires_operator_execution": true,
		"auth_context_contract":       manifest.AuthContextContract,
		"required_checks":             manifest.RequiredChecks,
		"raw_provider_input":          fixture.rawProviderInput,
	})
	_, err := prepareExternalAuditAppend(fixture.config(false))
	if err == nil || !strings.Contains(err.Error(), "sensitive-looking") {
		t.Fatalf("expected sensitive manifest rejection, got %v", err)
	}
}

func TestExternalAuditAppendRejectsDisallowedAttributes(t *testing.T) {
	fixture := newAuditAppendFixture(t)
	manifest := fixture.manifest
	manifest.AttributesJSON = json.RawMessage(`{"raw_provider_body":"secret"}`)
	manifest.AttributesSHA256 = sha256Ref([]byte(compactJSON(manifest.AttributesJSON)))
	writeJSON(t, fixture.manifestPath, manifest)
	_, err := prepareExternalAuditAppend(fixture.config(false))
	if err == nil || !strings.Contains(err.Error(), "disallowed key") {
		t.Fatalf("expected disallowed attribute rejection, got %v", err)
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

func writeRawJSON(t *testing.T, path string, value any) {
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

type auditAppendFixture struct {
	dir              string
	manifest         externalAuditAppendManifest
	manifestPath     string
	attributesJSON   string
	attributesSHA    string
	rawProviderInput string
}

func newAuditAppendFixture(t *testing.T) auditAppendFixture {
	t.Helper()
	dir := t.TempDir()
	attributes := `{"proposal_id":"proposal-fresh-1","approval_id":"approval-fresh-1","prepared_audit_id":"audit-fresh-1","execution_id":"execution-redrive-1","result_id":"result-redrive-1","source_ref":"provider-failure-1","operator_mode":"provider-replay-redrive","status":"SUCCEEDED","target_ref_hash":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`
	attributesJSON := compactJSON(json.RawMessage(attributes))
	manifest := externalAuditAppendManifest{
		SchemaVersion:     "nexusim.action_executor.external_audit_append.v1",
		ManifestID:        "action-executor-audit-append-1",
		SourceManifestID:  "provider-replay-redrive-invocation-1",
		ExecutesAppend:    false,
		MutatesAudit:      false,
		DirectAppend:      false,
		RequiresExecution: true,
		AuditStream:       "security",
		SourceService:     "action-executor",
		SourceEventID:     "execution-redrive-1",
		RecordType:        "ACTION_PROVIDER_REDRIVE",
		ActorRef:          "service:action-executor",
		SubjectRef:        "workflow:workflow-provider-replay-1",
		ResourceRef:       "hash:sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Action:            "REDRIVE_PROVIDER_FAILURE",
		Outcome:           "SUCCEEDED",
		ReasonCode:        "PROVIDER_REPLAY_APPROVED",
		RiskLevel:         "HIGH",
		OccurredAtUnixMs:  time.Date(2026, 6, 25, 3, 0, 0, 0, time.UTC).UnixMilli(),
		AttributesJSON:    json.RawMessage(attributesJSON),
		AttributesSHA256:  sha256Ref([]byte(attributesJSON)),
		IdempotencyKey:    "action-executor:audit:execution-redrive-1",
		CorrelationID:     "corr-provider-replay",
		CausationID:       "provider-replay-redrive-invocation-1",
		TraceID:           "trace-provider-replay",
		RequiredChecks: []string{
			"source_execution_audit_low_sensitive",
			"no_raw_provider_artifacts",
			"audit_service_append_only",
			"idempotency_key_present",
		},
		ForbiddenContents: []string{
			"raw_provider_input",
			"raw_provider_output",
			"input_json",
		},
	}
	manifest.AuthContextContract.TenantID = "tenant-provider-replay"
	manifest.AuthContextContract.TraceID = "trace-provider-replay"
	manifestPath := filepath.Join(dir, "action-executor-audit-append.json")
	writeJSON(t, manifestPath, manifest)
	return auditAppendFixture{
		dir:              dir,
		manifest:         manifest,
		manifestPath:     manifestPath,
		attributesJSON:   attributesJSON,
		attributesSHA:    manifest.AttributesSHA256,
		rawProviderInput: `{"token":"secret-provider-payload"}`,
	}
}

func (fixture auditAppendFixture) config(execute bool) config {
	return config{
		mode:            "external-audit-append",
		auditTarget:     "127.0.0.1:10700",
		requestTimeout:  time.Second,
		auditManifest:   fixture.manifestPath,
		operatorUserID:  "operator-user",
		operatorDevice:  "operator-device",
		operatorSession: "operator-session",
		traceID:         "trace-provider-replay",
		requestID:       "request-audit-append",
		execute:         execute,
	}
}

type fakeAuditClient struct {
	requests []*auditv1.AppendAuditRecordRequest
	response *auditv1.AppendAuditRecordResponse
}

func (client *fakeAuditClient) AppendAuditRecord(
	_ context.Context,
	request *auditv1.AppendAuditRecordRequest,
	_ ...grpc.CallOption,
) (*auditv1.AppendAuditRecordResponse, error) {
	client.requests = append(client.requests, request)
	if client.response != nil {
		return client.response, nil
	}
	return &auditv1.AppendAuditRecordResponse{
		Record: &auditv1.AuditRecord{
			AuditId:        "audit-action-executor-1",
			RecordHash:     "record-hash-1",
			IdempotencyKey: request.GetIdempotencyKey(),
		},
	}, nil
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
