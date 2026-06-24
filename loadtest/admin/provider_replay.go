package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	adminv1 "github.com/qsyy0921/IM/api/proto/nexusim/admin/v1"
)

const (
	providerReplayOperationType  = "PROVIDER_REPLAY_REQUEST"
	providerReplayWorkflowType   = "REPAIR_APPROVAL"
	providerReplayTargetService  = "action-executor"
	providerReplayEntrypoint     = "RedriveProviderFailure"
	providerReplayApprovalPolicy = "admin.workflow.provider_replay.v1"
	providerReplayPayloadSchema  = "admin.provider_replay_request.v1"
	providerReplayHandoffKind    = "action-executor.provider-failure.replay-admin-workflow-handoff"
)

type providerReplayHandoffDocument struct {
	Kind                    string                          `json:"kind"`
	HandoffContract         *providerReplayHandoffContract  `json:"handoff_contract"`
	AdminOperationRequests  []providerReplayAdminRequest    `json:"admin_operation_requests"`
	WorkflowHandoffRequests []providerReplayWorkflowRequest `json:"workflow_handoff_requests"`
}

type providerReplayHandoffContract struct {
	AdminOperationType     string   `json:"admin_operation_type"`
	WorkflowType           string   `json:"workflow_type"`
	TargetService          string   `json:"target_service"`
	TargetOperation        string   `json:"target_operation"`
	RedriveEntrypoint      string   `json:"redrive_entrypoint"`
	ApprovalPolicyRef      string   `json:"approval_policy_ref"`
	PayloadSchemaVersion   string   `json:"payload_schema_version"`
	DirectExecutionAllowed bool     `json:"direct_execution_allowed"`
	SourceDLQImmutable     bool     `json:"source_dlq_immutable"`
	Requires               []string `json:"requires"`
}

type providerReplayAdminRequest struct {
	AuthTenantID           string         `json:"auth_tenant_id"`
	OperatorRef            string         `json:"operator_ref"`
	OperatorRole           string         `json:"operator_role"`
	OperationType          string         `json:"operation_type"`
	TargetRefHash          string         `json:"target_ref_hash"`
	RiskLevel              string         `json:"risk_level"`
	PayloadSchemaVersion   string         `json:"payload_schema_version"`
	OperationPayload       map[string]any `json:"operation_payload"`
	OperationPayloadHash   string         `json:"operation_payload_hash"`
	ReasonRef              string         `json:"reason_ref"`
	EvidenceRefs           []string       `json:"evidence_refs"`
	IdempotencyKey         string         `json:"idempotency_key"`
	CorrelationID          string         `json:"correlation_id,omitempty"`
	CausationID            string         `json:"causation_id,omitempty"`
	TraceID                string         `json:"trace_id,omitempty"`
	ExpectedWorkflowPolicy string         `json:"expected_workflow_policy"`
}

type providerReplayWorkflowRequest struct {
	WorkflowType         string   `json:"workflow_type"`
	RequesterService     string   `json:"requester_service"`
	TargetService        string   `json:"target_service"`
	TargetOperation      string   `json:"target_operation"`
	RiskLevel            string   `json:"risk_level"`
	TargetRefHash        string   `json:"target_ref_hash"`
	PayloadSchemaVersion string   `json:"payload_schema_version"`
	PayloadRefHash       string   `json:"payload_ref_hash"`
	ApprovalPolicyRef    string   `json:"approval_policy_ref"`
	ReasonRef            string   `json:"reason_ref"`
	EvidenceRefs         []string `json:"evidence_refs"`
	IdempotencyKey       string   `json:"idempotency_key"`
	CorrelationID        string   `json:"correlation_id,omitempty"`
	CausationID          string   `json:"causation_id,omitempty"`
	TraceID              string   `json:"trace_id,omitempty"`
}

func submitProviderReplayHandoff(
	ctx context.Context,
	cfg config,
	client adminv1.AdminServiceClient,
) ([]operationSummary, error) {
	requests, err := providerReplayAdminRequestsFromHandoffFile(cfg.providerReplayHandoffFile)
	if err != nil {
		return nil, err
	}
	summaries := make([]operationSummary, 0, len(requests))
	for _, request := range requests {
		payloadJSON, err := providerReplayPayloadJSON(request)
		if err != nil {
			return nil, err
		}
		response, err := client.CreateAdminOperation(ctx, &adminv1.CreateAdminOperationRequest{
			AuthContext:          providerReplayAuthContext(cfg, request),
			OperatorRef:          request.OperatorRef,
			OperatorRole:         request.OperatorRole,
			OperationType:        request.OperationType,
			TargetRefHash:        request.TargetRefHash,
			RiskLevel:            request.RiskLevel,
			PayloadSchemaVersion: request.PayloadSchemaVersion,
			OperationPayloadJson: payloadJSON,
			ReasonRef:            request.ReasonRef,
			EvidenceRefs:         append([]string(nil), request.EvidenceRefs...),
			IdempotencyKey:       request.IdempotencyKey,
			CorrelationId:        providerReplayFirstNonEmpty(request.CorrelationID, cfg.requestID),
			CausationId:          providerReplayFirstNonEmpty(request.CausationID, providerReplayOperationType),
			TraceId:              providerReplayFirstNonEmpty(request.TraceID, cfg.traceID),
		})
		if err != nil {
			return nil, fmt.Errorf("create provider replay admin operation: %w", err)
		}
		if summary := summarizeOperation(response.GetOperation()); summary != nil {
			summaries = append(summaries, *summary)
		}
	}
	return summaries, nil
}

func providerReplayAdminRequestsFromHandoffFile(path string) ([]providerReplayAdminRequest, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("--provider-replay-handoff-file is required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read provider replay handoff file: %w", err)
	}
	return providerReplayAdminRequestsFromHandoff(content)
}

func providerReplayAdminRequestsFromHandoff(content []byte) ([]providerReplayAdminRequest, error) {
	var document providerReplayHandoffDocument
	if err := json.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("decode provider replay handoff: %w", err)
	}
	if err := validateProviderReplayHandoffDocument(document); err != nil {
		return nil, err
	}
	requests := make([]providerReplayAdminRequest, 0, len(document.AdminOperationRequests))
	for _, request := range document.AdminOperationRequests {
		if err := validateProviderReplayAdminRequest(request); err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, nil
}

func validateProviderReplayHandoffDocument(document providerReplayHandoffDocument) error {
	if strings.TrimSpace(document.Kind) != providerReplayHandoffKind {
		return errors.New("provider replay handoff kind is not supported")
	}
	contract := document.HandoffContract
	if contract == nil {
		return errors.New("provider replay handoff contract is required")
	}
	if contract.AdminOperationType != providerReplayOperationType ||
		contract.WorkflowType != providerReplayWorkflowType ||
		contract.TargetService != providerReplayTargetService ||
		contract.TargetOperation != providerReplayOperationType ||
		contract.RedriveEntrypoint != providerReplayEntrypoint ||
		contract.ApprovalPolicyRef != providerReplayApprovalPolicy ||
		contract.PayloadSchemaVersion != providerReplayPayloadSchema ||
		contract.DirectExecutionAllowed ||
		!contract.SourceDLQImmutable {
		return errors.New("provider replay handoff contract does not match admin/workflow safety boundary")
	}
	if len(document.AdminOperationRequests) == 0 {
		return errors.New("provider replay handoff contains no admin operation requests")
	}
	return nil
}

func validateProviderReplayAdminRequest(request providerReplayAdminRequest) error {
	if strings.TrimSpace(request.AuthTenantID) == "" ||
		strings.TrimSpace(request.OperatorRef) == "" ||
		strings.TrimSpace(request.OperatorRole) == "" ||
		strings.TrimSpace(request.TargetRefHash) == "" ||
		strings.TrimSpace(request.ReasonRef) == "" ||
		strings.TrimSpace(request.IdempotencyKey) == "" {
		return errors.New("provider replay admin request is incomplete")
	}
	if request.OperationType != providerReplayOperationType ||
		request.PayloadSchemaVersion != providerReplayPayloadSchema ||
		request.ExpectedWorkflowPolicy != providerReplayApprovalPolicy {
		return errors.New("provider replay admin request has unsupported operation contract")
	}
	if strings.ToUpper(strings.TrimSpace(request.RiskLevel)) != "HIGH" &&
		strings.ToUpper(strings.TrimSpace(request.RiskLevel)) != "CRITICAL" {
		return errors.New("provider replay admin request risk level must be high or critical")
	}
	if len(request.EvidenceRefs) == 0 {
		return errors.New("provider replay admin request evidence refs are required")
	}
	payloadJSON, err := providerReplayPayloadJSON(request)
	if err != nil {
		return err
	}
	if request.OperationPayloadHash != "sha256:"+providerReplaySHA256(payloadJSON) {
		return errors.New("provider replay admin request payload hash mismatch")
	}
	if !providerReplayLowSensitiveJSON(payloadJSON) ||
		!providerReplayLowSensitiveValue(request.OperatorRef) ||
		!providerReplayLowSensitiveValue(request.ReasonRef) ||
		!providerReplayLowSensitiveValue(request.TargetRefHash) ||
		!providerReplayLowSensitiveValue(request.CorrelationID) ||
		!providerReplayLowSensitiveValue(request.CausationID) ||
		!providerReplayLowSensitiveValue(request.TraceID) {
		return errors.New("provider replay admin request contains sensitive refs")
	}
	for _, ref := range request.EvidenceRefs {
		if !providerReplayLowSensitiveValue(ref) {
			return errors.New("provider replay admin request evidence ref is sensitive")
		}
	}
	return nil
}

func providerReplayPayloadJSON(request providerReplayAdminRequest) (string, error) {
	if len(request.OperationPayload) == 0 {
		return "", errors.New("provider replay admin request payload is required")
	}
	if request.OperationPayload["redrive_entrypoint"] != providerReplayEntrypoint ||
		request.OperationPayload["source_dlq_immutable"] != true ||
		request.OperationPayload["direct_execution_allowed"] != false ||
		request.OperationPayload["requires_fresh_proposal"] != true ||
		request.OperationPayload["requires_fresh_approval"] != true ||
		request.OperationPayload["requires_prepared_audit"] != true ||
		request.OperationPayload["requires_new_input"] != true ||
		request.OperationPayload["requires_reason_sha256"] != true ||
		strings.TrimSpace(fmt.Sprint(request.OperationPayload["provider_failure_ref_hash"])) == "" ||
		strings.TrimSpace(fmt.Sprint(request.OperationPayload["source_execution_ref_hash"])) == "" ||
		strings.TrimSpace(fmt.Sprint(request.OperationPayload["source_result_ref_hash"])) == "" ||
		strings.TrimSpace(fmt.Sprint(request.OperationPayload["replay_candidate_id"])) == "" {
		return "", errors.New("provider replay admin request payload violates redrive safety flags")
	}
	encoded, err := json.Marshal(request.OperationPayload)
	if err != nil {
		return "", fmt.Errorf("encode provider replay admin request payload: %w", err)
	}
	return string(encoded), nil
}

func providerReplayAuthContext(cfg config, request providerReplayAdminRequest) *adminv1.AuthContext {
	return &adminv1.AuthContext{
		TenantId:    request.AuthTenantID,
		UserId:      cfg.userID,
		ServiceName: "admin-provider-replay-operator",
		InstanceRef: cfg.instanceRef,
		TraceId:     providerReplayFirstNonEmpty(request.TraceID, cfg.traceID),
		RequestId:   cfg.requestID,
	}
}

func providerReplaySHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func providerReplayFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func providerReplayLowSensitiveJSON(value string) bool {
	return providerReplayLowSensitiveValue(value)
}

func providerReplayLowSensitiveValue(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return true
	}
	for _, marker := range []string{
		"password",
		"token",
		"secret",
		"api_key",
		"apikey",
		"private://",
		"raw:",
		"dsn=",
		"postgres://",
		"http://",
		"https://",
		"message_body",
		"provider_body",
		"prompt",
		"input_json",
		"output_json",
	} {
		if strings.Contains(value, marker) {
			return false
		}
	}
	return len(value) <= 4096
}
