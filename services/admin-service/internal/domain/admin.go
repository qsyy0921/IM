package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

var allowedOperationTypes = map[string]bool{
	"TENANT_CREATE":                   true,
	"TENANT_UPDATE":                   true,
	"TENANT_DISABLE":                  true,
	"USER_BAN":                        true,
	"USER_UNBAN":                      true,
	"DEVICE_REVOKE":                   true,
	"SESSION_REVOKE":                  true,
	"CONTACT_REQUEST_REVIEW":          true,
	"CONTACT_PRIVACY_POLICY_CHANGE":   true,
	"POLICY_RULE_CHANGE":              true,
	"REBAC_RELATION_CHANGE":           true,
	"TENANT_QUOTA_CHANGE":             true,
	"CONFIG_PUBLISH":                  true,
	"CONFIG_ROLLBACK":                 true,
	"REPAIR_REQUEST":                  true,
	"AUDIT_EXPORT_REQUEST":            true,
	"NOTIFICATION_SUPPRESSION_CHANGE": true,
}

var allowedPayloadKeys = map[string]bool{
	"target_user_ref":          true,
	"device_ref":               true,
	"session_ref":              true,
	"config_bundle_key_hash":   true,
	"config_version":           true,
	"quota_rps":                true,
	"quota_burst":              true,
	"policy_rule_ref":          true,
	"rebac_relation_ref":       true,
	"repair_mode":              true,
	"audit_export_filter_hash": true,
	"suppression_rule_ref":     true,
	"tenant_ref":               true,
	"workflow_ref":             true,
	"downstream_request_ref":   true,
}

var forbiddenPayloadKeyFragments = []string{
	"password",
	"token",
	"totp",
	"recovery",
	"secret",
	"private_key",
	"object_key",
	"dsn",
	"sql",
	"prompt",
	"message_body",
	"evidence_pack",
	"provider_body",
	"reason_raw",
}

type PreparedOperation struct {
	Command     types.CreateAdminOperationCommand
	OperationID string
	CommandHash string
	PayloadJSON string
	PayloadHash string
	CreatedAt   time.Time
}

type PreparedApproval struct {
	Command     types.ApproveAdminOperationCommand
	ApprovalID  string
	CommandHash string
	CreatedAt   time.Time
}

func PrepareCreate(command types.CreateAdminOperationCommand, operationID string, now time.Time) (PreparedOperation, error) {
	command.OperatorRole = normalizeUpper(command.OperatorRole)
	command.OperationType = normalizeUpper(command.OperationType)
	command.RiskLevel = normalizeUpper(command.RiskLevel)
	if !command.AuthContext.Valid() {
		return PreparedOperation{}, types.NewInvalidArgument("auth context is required")
	}
	if strings.TrimSpace(command.OperatorRef) == "" || !isAdminRole(command.OperatorRole) {
		return PreparedOperation{}, types.NewPermissionDenied("operator role is not allowed")
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" {
		return PreparedOperation{}, types.NewInvalidArgument("idempotency key is required")
	}
	if strings.TrimSpace(operationID) == "" {
		return PreparedOperation{}, types.NewInvalidArgument("operation id is required")
	}
	if !allowedOperationTypes[command.OperationType] {
		return PreparedOperation{}, types.NewInvalidArgument("operation type is unsupported")
	}
	if !validRisk(command.RiskLevel) {
		return PreparedOperation{}, types.NewInvalidArgument("risk level is unsupported")
	}
	if strings.TrimSpace(command.TargetRefHash) == "" || !looksLikeHashRef(command.TargetRefHash) {
		return PreparedOperation{}, types.NewInvalidArgument("target_ref_hash is required")
	}
	if strings.TrimSpace(command.PayloadSchemaVersion) == "" {
		return PreparedOperation{}, types.NewInvalidArgument("payload schema version is required")
	}
	if (command.RiskLevel == types.RiskLevelHigh || command.RiskLevel == types.RiskLevelCritical) &&
		(strings.TrimSpace(command.ReasonRef) == "" || len(command.EvidenceRefs) == 0) {
		return PreparedOperation{}, types.NewInvalidArgument("high risk operation requires reason and evidence refs")
	}
	payload, err := NormalizePayload(command.OperationPayloadJSON)
	if err != nil {
		return PreparedOperation{}, err
	}
	command.EvidenceRefs = normalizeRefs(command.EvidenceRefs)
	command.TraceID = firstNonEmpty(command.TraceID, command.AuthContext.TraceID)
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	hashInput := map[string]any{
		"tenant_id":       string(command.AuthContext.TenantID),
		"operator_ref":    command.OperatorRef,
		"operation_type":  command.OperationType,
		"target_ref_hash": command.TargetRefHash,
		"risk_level":      command.RiskLevel,
		"payload_schema":  command.PayloadSchemaVersion,
		"payload_json":    payload,
		"reason_ref":      command.ReasonRef,
		"evidence_refs":   command.EvidenceRefs,
	}
	return PreparedOperation{
		Command:     command,
		OperationID: operationID,
		CommandHash: HashCanonical(hashInput),
		PayloadJSON: payload,
		PayloadHash: HashText(payload),
		CreatedAt:   now.UTC(),
	}, nil
}

func PrepareApproval(command types.ApproveAdminOperationCommand, approvalID string, now time.Time) (PreparedApproval, error) {
	command.ApproverRole = normalizeUpper(command.ApproverRole)
	command.Decision = normalizeUpper(command.Decision)
	if !command.AuthContext.Valid() {
		return PreparedApproval{}, types.NewInvalidArgument("auth context is required")
	}
	if strings.TrimSpace(command.OperationID) == "" {
		return PreparedApproval{}, types.NewInvalidArgument("operation id is required")
	}
	if strings.TrimSpace(command.ApproverRef) == "" || !isAdminRole(command.ApproverRole) {
		return PreparedApproval{}, types.NewPermissionDenied("approver role is not allowed")
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" {
		return PreparedApproval{}, types.NewInvalidArgument("idempotency key is required")
	}
	if strings.TrimSpace(approvalID) == "" {
		return PreparedApproval{}, types.NewInvalidArgument("approval id is required")
	}
	if command.Decision != types.DecisionApprove && command.Decision != types.DecisionReject {
		return PreparedApproval{}, types.NewInvalidArgument("approval decision is unsupported")
	}
	command.EvidenceRefs = normalizeRefs(command.EvidenceRefs)
	command.TraceID = firstNonEmpty(command.TraceID, command.AuthContext.TraceID)
	hashInput := map[string]any{
		"tenant_id":           string(command.AuthContext.TenantID),
		"operation_id":        command.OperationID,
		"approver_ref":        command.ApproverRef,
		"decision":            command.Decision,
		"approval_policy_ref": command.ApprovalPolicyRef,
		"reason_ref":          command.ReasonRef,
		"evidence_refs":       command.EvidenceRefs,
	}
	return PreparedApproval{
		Command:     command,
		ApprovalID:  approvalID,
		CommandHash: HashCanonical(hashInput),
		CreatedAt:   now.UTC(),
	}, nil
}

func OperationFromPrepared(prepared PreparedOperation) types.AdminOperation {
	return types.AdminOperation{
		TenantID:             prepared.Command.AuthContext.TenantID,
		OperationID:          prepared.OperationID,
		IdempotencyKey:       prepared.Command.IdempotencyKey,
		CommandHash:          prepared.CommandHash,
		OperationType:        prepared.Command.OperationType,
		TargetRefHash:        prepared.Command.TargetRefHash,
		RiskLevel:            prepared.Command.RiskLevel,
		PayloadSchemaVersion: prepared.Command.PayloadSchemaVersion,
		PayloadJSON:          prepared.PayloadJSON,
		PayloadHash:          prepared.PayloadHash,
		ReasonRef:            strings.TrimSpace(prepared.Command.ReasonRef),
		EvidenceRefs:         prepared.Command.EvidenceRefs,
		Status:               types.OperationStatusSubmitted,
		RequestedBy:          strings.TrimSpace(prepared.Command.OperatorRef),
		RequestedAt:          prepared.CreatedAt,
		CorrelationID:        prepared.Command.CorrelationID,
		CausationID:          prepared.Command.CausationID,
		TraceID:              prepared.Command.TraceID,
		UpdatedAt:            prepared.CreatedAt,
	}
}

func ApprovalFromPrepared(prepared PreparedApproval, tenantID types.TenantID) types.AdminApproval {
	return types.AdminApproval{
		TenantID:          tenantID,
		ApprovalID:        prepared.ApprovalID,
		OperationID:       prepared.Command.OperationID,
		IdempotencyKey:    prepared.Command.IdempotencyKey,
		CommandHash:       prepared.CommandHash,
		ApproverRef:       strings.TrimSpace(prepared.Command.ApproverRef),
		Decision:          prepared.Command.Decision,
		ApprovalPolicyRef: strings.TrimSpace(prepared.Command.ApprovalPolicyRef),
		ReasonRef:         strings.TrimSpace(prepared.Command.ReasonRef),
		EvidenceRefs:      prepared.Command.EvidenceRefs,
		CreatedAt:         prepared.CreatedAt,
	}
}

func NormalizePayload(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", types.NewInvalidArgument("operation payload must be JSON object")
	}
	for key, value := range payload {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if !allowedPayloadKeys[normalized] {
			return "", types.NewInvalidArgument("operation payload contains unsupported field")
		}
		for _, fragment := range forbiddenPayloadKeyFragments {
			if strings.Contains(normalized, fragment) {
				return "", types.NewInvalidArgument("operation payload contains sensitive field")
			}
		}
		if !lowSensitiveValue(value) {
			return "", types.NewInvalidArgument("operation payload contains unsupported value")
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewInvalidArgument("operation payload is invalid")
	}
	return string(encoded), nil
}

func ValidateApprovalTransition(operation types.AdminOperation, approval types.AdminApproval) error {
	if operation.Status != types.OperationStatusSubmitted {
		return types.NewFailedPrecondition("operation is not waiting for approval")
	}
	if approval.Decision == types.DecisionApprove &&
		(operation.RiskLevel == types.RiskLevelHigh || operation.RiskLevel == types.RiskLevelCritical) &&
		operation.RequestedBy == approval.ApproverRef {
		return types.NewPermissionDenied("approval violates separation of duty")
	}
	return nil
}

func HashCanonical(value any) string {
	encoded, _ := json.Marshal(value)
	return HashText(string(encoded))
}

func HashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func normalizeUpper(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func isAdminRole(role string) bool {
	switch role {
	case "OWNER", "SUPER_ADMIN", "ADMIN", "OPERATOR":
		return true
	default:
		return false
	}
}

func validRisk(risk string) bool {
	switch risk {
	case types.RiskLevelLow, types.RiskLevelMedium, types.RiskLevelHigh, types.RiskLevelCritical:
		return true
	default:
		return false
	}
}

func looksLikeHashRef(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "sha256:") || strings.HasPrefix(value, "hash:")
}

func normalizeRefs(refs []string) []string {
	clean := make([]string, 0, len(refs))
	seen := map[string]bool{}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		clean = append(clean, ref)
	}
	sort.Strings(clean)
	return clean
}

func lowSensitiveValue(value any) bool {
	switch typed := value.(type) {
	case string:
		lower := strings.ToLower(typed)
		for _, marker := range []string{"password", "token", "secret", "-----begin", "s3://", "http://", "https://"} {
			if strings.Contains(lower, marker) {
				return false
			}
		}
		return len(typed) <= 256
	case float64, bool, nil:
		return true
	case []any:
		if len(typed) > 16 {
			return false
		}
		for _, item := range typed {
			if !lowSensitiveValue(item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
