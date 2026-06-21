package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/audit-service/internal/types"
)

type PreparedRecord struct {
	Command           types.AppendAuditRecordCommand
	CanonicalJSON     string
	CanonicalJSONHash string
	CommandHash       string
}

type PreparedExport struct {
	Command     types.CreateAuditExportCommand
	CommandHash string
}

func PrepareRecord(command types.AppendAuditRecordCommand) (PreparedRecord, error) {
	command = command.Normalized()
	if err := command.Validate(); err != nil {
		return PreparedRecord{}, err
	}
	canonical, err := CanonicalJSON(command)
	if err != nil {
		return PreparedRecord{}, err
	}
	canonicalHash := SHA256Hex(canonical)
	commandHash := SHA256Hex(strings.Join([]string{
		string(command.AuthContext.TenantID),
		command.AuditStream,
		command.SourceService,
		command.SourceEventID,
		command.RecordType,
		command.ActorRef,
		command.SubjectRef,
		command.ResourceRef,
		command.Action,
		command.Outcome,
		command.ReasonCode,
		command.RiskLevel,
		command.OccurredAt.UTC().Format(time.RFC3339Nano),
		command.AttributesJSON,
		command.IdempotencyKey,
		command.CorrelationID,
		command.CausationID,
		command.TraceID,
	}, "\x00"))
	return PreparedRecord{
		Command:           command,
		CanonicalJSON:     canonical,
		CanonicalJSONHash: canonicalHash,
		CommandHash:       commandHash,
	}, nil
}

func PrepareExport(command types.CreateAuditExportCommand) (PreparedExport, error) {
	command = command.Normalized()
	if err := command.Validate(); err != nil {
		return PreparedExport{}, err
	}
	commandHash := SHA256Hex(strings.Join([]string{
		string(command.AuthContext.TenantID),
		command.AuditStream,
		command.RecordType,
		command.SourceService,
		command.FilterHash,
		command.RedactionProfile,
		command.RequestedByRef,
		command.IdempotencyKey,
		command.CorrelationID,
		command.CausationID,
		command.TraceID,
	}, "\x00"))
	return PreparedExport{Command: command, CommandHash: commandHash}, nil
}

func CanonicalJSON(command types.AppendAuditRecordCommand) (string, error) {
	var attributes map[string]any
	if err := json.Unmarshal([]byte(command.AttributesJSON), &attributes); err != nil {
		return "", types.NewInvalidArgument("attributes_json must be valid json")
	}
	payload := map[string]any{
		"tenant_id":       string(command.AuthContext.TenantID),
		"audit_stream":    command.AuditStream,
		"source_service":  command.SourceService,
		"source_event_id": command.SourceEventID,
		"record_type":     command.RecordType,
		"actor_ref":       command.ActorRef,
		"subject_ref":     command.SubjectRef,
		"resource_ref":    command.ResourceRef,
		"action":          command.Action,
		"outcome":         command.Outcome,
		"reason_code":     command.ReasonCode,
		"risk_level":      command.RiskLevel,
		"occurred_at":     command.OccurredAt.UTC().Format(time.RFC3339Nano),
		"attributes":      attributes,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewInvalidArgument("audit record canonicalization failed")
	}
	return string(encoded), nil
}

func RecordHash(previousRecordHash string, canonicalJSONHash string, tenantID types.TenantID, auditStream string, auditID string) string {
	return SHA256Hex(strings.Join([]string{
		strings.TrimSpace(previousRecordHash),
		strings.TrimSpace(canonicalJSONHash),
		string(tenantID),
		strings.TrimSpace(auditStream),
		strings.TrimSpace(auditID),
	}, "\x00"))
}

func SHA256Hex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
