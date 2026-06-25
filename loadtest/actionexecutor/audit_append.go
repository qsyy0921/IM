package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	auditv1 "github.com/qsyy0921/IM/api/proto/nexusim/audit/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
)

const maxAuditManifestBytes = 128 * 1024

var allowedAuditAttributeKeys = map[string]struct{}{
	"proposal_id":             {},
	"approval_id":             {},
	"prepared_audit_id":       {},
	"execution_id":            {},
	"result_id":               {},
	"failure_class":           {},
	"provider_class":          {},
	"operator_mode":           {},
	"repair_outcome":          {},
	"source_ref":              {},
	"event_type":              {},
	"operation_id":            {},
	"operation_type":          {},
	"target_ref_hash":         {},
	"risk_level":              {},
	"status":                  {},
	"requested_by_hash":       {},
	"approved_by_hash":        {},
	"payload_hash":            {},
	"payload_schema_version":  {},
	"reason_ref":              {},
	"decision":                {},
	"downstream_service":      {},
	"downstream_request_ref":  {},
	"aggregate_version":       {},
	"partition_key":           {},
	"policy_decision_id":      {},
	"notification_key":        {},
	"request_key":             {},
	"export_id":               {},
	"record_count":            {},
	"message_key":             {},
	"conversation_key":        {},
	"session_key":             {},
	"device_key":              {},
	"compensation_reason_ref": {},
}

type externalAuditAppendManifest struct {
	SchemaVersion       string          `json:"schema_version"`
	ManifestID          string          `json:"manifest_id"`
	SourceManifestID    string          `json:"source_manifest_id"`
	ExecutesAppend      bool            `json:"executes_append"`
	MutatesAudit        bool            `json:"mutates_audit_service"`
	DirectAppend        bool            `json:"direct_append_allowed"`
	RequiresExecution   bool            `json:"requires_operator_execution"`
	AuditStream         string          `json:"audit_stream"`
	SourceService       string          `json:"source_service"`
	SourceEventID       string          `json:"source_event_id"`
	RecordType          string          `json:"record_type"`
	ActorRef            string          `json:"actor_ref"`
	SubjectRef          string          `json:"subject_ref"`
	ResourceRef         string          `json:"resource_ref"`
	Action              string          `json:"action"`
	Outcome             string          `json:"outcome"`
	ReasonCode          string          `json:"reason_code"`
	RiskLevel           string          `json:"risk_level"`
	OccurredAtUnixMs    int64           `json:"occurred_at_unix_ms"`
	AttributesJSON      json.RawMessage `json:"attributes_json"`
	AttributesSHA256    string          `json:"attributes_sha256"`
	IdempotencyKey      string          `json:"idempotency_key"`
	CorrelationID       string          `json:"correlation_id"`
	CausationID         string          `json:"causation_id"`
	TraceID             string          `json:"trace_id"`
	AuthContextContract struct {
		TenantID string `json:"tenant_id"`
		TraceID  string `json:"trace_id"`
	} `json:"auth_context_contract"`
	RequiredChecks    []string `json:"required_checks"`
	ForbiddenContents []string `json:"forbidden_contents"`
}

type preparedAuditAppend struct {
	Manifest         externalAuditAppendManifest
	Request          *auditv1.AppendAuditRecordRequest
	AttributesSHA256 string
	Verified         []string
}

type auditAppendCommandResult struct {
	Mode             string                `json:"mode"`
	AuditTarget      string                `json:"audit_target,omitempty"`
	ManifestID       string                `json:"manifest_id"`
	SourceManifestID string                `json:"source_manifest_id,omitempty"`
	Request          auditRequestSummary   `json:"request"`
	Response         *auditResponseSummary `json:"response,omitempty"`
	ExecutedAppend   bool                  `json:"executed_append"`
	Verified         []string              `json:"verified"`
	CheckedAt        time.Time             `json:"checked_at"`
}

type auditRequestSummary struct {
	TenantID         string `json:"tenant_id"`
	UserID           string `json:"user_id"`
	DeviceID         string `json:"device_id"`
	AuditStream      string `json:"audit_stream"`
	SourceService    string `json:"source_service"`
	SourceEventID    string `json:"source_event_id"`
	RecordType       string `json:"record_type"`
	ResourceRef      string `json:"resource_ref"`
	Action           string `json:"action"`
	Outcome          string `json:"outcome"`
	ReasonCode       string `json:"reason_code"`
	RiskLevel        string `json:"risk_level"`
	AttributesSHA256 string `json:"attributes_sha256"`
	IdempotencyKey   string `json:"idempotency_key"`
	CorrelationID    string `json:"correlation_id,omitempty"`
	CausationID      string `json:"causation_id,omitempty"`
	TraceID          string `json:"trace_id,omitempty"`
	OccurredAtUnixMs int64  `json:"occurred_at_unix_ms"`
}

type auditResponseSummary struct {
	AuditID            string `json:"audit_id"`
	RecordHash         string `json:"record_hash"`
	PreviousRecordHash string `json:"previous_record_hash,omitempty"`
	IdempotencyKey     string `json:"idempotency_key"`
}

func runExternalAuditAppend(ctx context.Context, cfg config, out io.Writer, client auditClient) error {
	prepared, err := prepareExternalAuditAppend(cfg)
	if err != nil {
		return err
	}
	result := auditAppendCommandResult{
		Mode:             cfg.mode,
		AuditTarget:      cfg.auditTarget,
		ManifestID:       prepared.Manifest.ManifestID,
		SourceManifestID: prepared.Manifest.SourceManifestID,
		Request:          summarizeAuditRequest(prepared),
		ExecutedAppend:   false,
		Verified:         prepared.Verified,
		CheckedAt:        time.Now().UTC(),
	}
	if cfg.execute {
		if cfg.auditTarget == "" {
			return errors.New("--audit-target is required when --execute is set")
		}
		if client == nil {
			dialOption, err := grpctls.DialOption(cfg.auditTLS, "audit-tls")
			if err != nil {
				return err
			}
			conn, err := grpc.NewClient("passthrough:///"+cfg.auditTarget, dialOption)
			if err != nil {
				return fmt.Errorf("dial audit-service: %w", err)
			}
			defer conn.Close()
			client = auditv1.NewAuditServiceClient(conn)
		}
		requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
		defer cancel()
		response, err := client.AppendAuditRecord(requestCtx, prepared.Request)
		if err != nil {
			return fmt.Errorf("append audit record: %w", err)
		}
		result.ExecutedAppend = true
		result.Response = summarizeAuditResponse(response)
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func prepareExternalAuditAppend(cfg config) (preparedAuditAppend, error) {
	manifestPath := firstNonEmpty(cfg.auditManifest, cfg.manifestPath)
	if manifestPath == "" {
		return preparedAuditAppend{}, errors.New("--audit-manifest is required")
	}
	if cfg.operatorUserID == "" {
		return preparedAuditAppend{}, errors.New("--operator-user-id is required")
	}
	if cfg.operatorDevice == "" {
		return preparedAuditAppend{}, errors.New("--operator-device-id is required")
	}
	if err := requireExternalFile(manifestPath, "audit-manifest"); err != nil {
		return preparedAuditAppend{}, err
	}
	manifest, raw, err := readAuditAppendManifest(manifestPath)
	if err != nil {
		return preparedAuditAppend{}, err
	}
	if err := validateAuditAppendManifest(manifest, raw); err != nil {
		return preparedAuditAppend{}, err
	}
	attributes := compactJSON(manifest.AttributesJSON)
	attributesSHA := sha256Ref([]byte(attributes))
	if attributesSHA != strings.ToLower(strings.TrimSpace(manifest.AttributesSHA256)) {
		return preparedAuditAppend{}, errors.New("attributes_json sha256 does not match manifest attributes_sha256")
	}
	request := &auditv1.AppendAuditRecordRequest{
		AuthContext: &auditv1.AuthContext{
			TenantId:  manifest.AuthContextContract.TenantID,
			UserId:    cfg.operatorUserID,
			DeviceId:  cfg.operatorDevice,
			SessionId: cfg.operatorSession,
			TraceId:   firstNonEmpty(cfg.traceID, manifest.AuthContextContract.TraceID, manifest.TraceID),
			RequestId: cfg.requestID,
		},
		AuditStream:      manifest.AuditStream,
		SourceService:    manifest.SourceService,
		SourceEventId:    manifest.SourceEventID,
		RecordType:       manifest.RecordType,
		ActorRef:         manifest.ActorRef,
		SubjectRef:       manifest.SubjectRef,
		ResourceRef:      manifest.ResourceRef,
		Action:           manifest.Action,
		Outcome:          manifest.Outcome,
		ReasonCode:       manifest.ReasonCode,
		RiskLevel:        manifest.RiskLevel,
		OccurredAtUnixMs: manifest.OccurredAtUnixMs,
		AttributesJson:   attributes,
		IdempotencyKey:   manifest.IdempotencyKey,
		CorrelationId:    manifest.CorrelationID,
		CausationId:      manifest.CausationID,
		TraceId:          firstNonEmpty(manifest.TraceID, manifest.AuthContextContract.TraceID, cfg.traceID),
	}
	return preparedAuditAppend{
		Manifest:         manifest,
		Request:          request,
		AttributesSHA256: attributesSHA,
		Verified: []string{
			"external_audit_manifest_contract_valid",
			"attributes_json_sha256_matches",
			"attributes_json_low_sensitive_keys_only",
			"audit_service_append_only",
		},
	}, nil
}

func readAuditAppendManifest(path string) (externalAuditAppendManifest, string, error) {
	data, err := readBoundedFile(path, maxAuditManifestBytes, "audit-manifest")
	if err != nil {
		return externalAuditAppendManifest{}, "", err
	}
	var manifest externalAuditAppendManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return externalAuditAppendManifest{}, "", fmt.Errorf("audit-manifest must be valid JSON: %w", err)
	}
	return manifest, string(data), nil
}

func validateAuditAppendManifest(manifest externalAuditAppendManifest, raw string) error {
	required := map[string]string{
		"schema_version":         manifest.SchemaVersion,
		"manifest_id":            manifest.ManifestID,
		"source_service":         manifest.SourceService,
		"source_event_id":        manifest.SourceEventID,
		"record_type":            manifest.RecordType,
		"resource_ref":           manifest.ResourceRef,
		"action":                 manifest.Action,
		"outcome":                manifest.Outcome,
		"idempotency_key":        manifest.IdempotencyKey,
		"attributes_sha256":      manifest.AttributesSHA256,
		"auth_context.tenant_id": manifest.AuthContextContract.TenantID,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("audit-manifest %s is required", field)
		}
	}
	if manifest.SchemaVersion != "nexusim.action_executor.external_audit_append.v1" {
		return errors.New("unsupported audit-manifest schema_version")
	}
	if manifest.SourceService != "action-executor" {
		return errors.New("audit-manifest source_service must be action-executor")
	}
	if manifest.ExecutesAppend || manifest.MutatesAudit || manifest.DirectAppend {
		return errors.New("audit-manifest must not claim it already appends or mutates audit-service")
	}
	if !manifest.RequiresExecution {
		return errors.New("audit-manifest must require operator execution")
	}
	if manifest.OccurredAtUnixMs <= 0 {
		return errors.New("audit-manifest occurred_at_unix_ms is required")
	}
	if !isSHA256Ref(manifest.AttributesSHA256) {
		return errors.New("audit-manifest attributes_sha256 must be sha256:<hex>")
	}
	if !json.Valid(manifest.AttributesJSON) {
		return errors.New("audit-manifest attributes_json must be valid JSON")
	}
	if err := validateAuditAttributes(manifest.AttributesJSON); err != nil {
		return err
	}
	for _, check := range []string{
		"source_execution_audit_low_sensitive",
		"no_raw_provider_artifacts",
		"audit_service_append_only",
		"idempotency_key_present",
	} {
		if !contains(manifest.RequiredChecks, check) {
			return fmt.Errorf("audit-manifest missing required check: %s", check)
		}
	}
	if containsSensitiveManifestText(raw) || auditManifestHasForbiddenTopLevelKey(raw) {
		return errors.New("audit-manifest contains sensitive-looking content")
	}
	return nil
}

func auditManifestHasForbiddenTopLevelKey(raw string) bool {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return true
	}
	for _, key := range []string{
		"input_json",
		"raw_input",
		"raw_output",
		"raw_provider_input",
		"raw_provider_output",
		"provider_body",
		"provider_error_body",
		"provider_artifact",
	} {
		if _, ok := envelope[key]; ok {
			return true
		}
	}
	return false
}

func validateAuditAttributes(raw json.RawMessage) error {
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("audit-manifest attributes_json must be object: %w", err)
	}
	if decoded == nil {
		return errors.New("audit-manifest attributes_json must be object")
	}
	for key := range decoded {
		normalized := strings.TrimSpace(key)
		if normalized == "" {
			return errors.New("audit-manifest attributes_json key is required")
		}
		if _, ok := allowedAuditAttributeKeys[normalized]; !ok {
			return fmt.Errorf("audit-manifest attributes_json contains disallowed key: %s", normalized)
		}
	}
	return nil
}

func compactJSON(raw json.RawMessage) string {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return string(raw)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return string(raw)
	}
	return string(encoded)
}

func summarizeAuditRequest(prepared preparedAuditAppend) auditRequestSummary {
	request := prepared.Request
	auth := request.GetAuthContext()
	return auditRequestSummary{
		TenantID:         auth.GetTenantId(),
		UserID:           auth.GetUserId(),
		DeviceID:         auth.GetDeviceId(),
		AuditStream:      request.GetAuditStream(),
		SourceService:    request.GetSourceService(),
		SourceEventID:    request.GetSourceEventId(),
		RecordType:       request.GetRecordType(),
		ResourceRef:      request.GetResourceRef(),
		Action:           request.GetAction(),
		Outcome:          request.GetOutcome(),
		ReasonCode:       request.GetReasonCode(),
		RiskLevel:        request.GetRiskLevel(),
		AttributesSHA256: prepared.AttributesSHA256,
		IdempotencyKey:   request.GetIdempotencyKey(),
		CorrelationID:    request.GetCorrelationId(),
		CausationID:      request.GetCausationId(),
		TraceID:          request.GetTraceId(),
		OccurredAtUnixMs: request.GetOccurredAtUnixMs(),
	}
}

func summarizeAuditResponse(response *auditv1.AppendAuditRecordResponse) *auditResponseSummary {
	record := response.GetRecord()
	if record == nil {
		return nil
	}
	return &auditResponseSummary{
		AuditID:            record.GetAuditId(),
		RecordHash:         record.GetRecordHash(),
		PreviousRecordHash: record.GetPreviousRecordHash(),
		IdempotencyKey:     record.GetIdempotencyKey(),
	}
}
