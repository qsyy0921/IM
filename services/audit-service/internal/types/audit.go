package types

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	DefaultAuditStream = "default"
	DefaultLimit       = 50
	MaxLimit           = 200
	MaxAttributesBytes = 16 * 1024

	AuditExportStatusPending   = "PENDING"
	AuditExportStatusRunning   = "RUNNING"
	AuditExportStatusCompleted = "COMPLETED"
	AuditExportStatusFailed    = "FAILED"
	AuditExportStatusCanceled  = "CANCELED"
)

var allowedAttributeKeys = map[string]struct{}{
	"message_key":                    {},
	"conversation_key":               {},
	"session_key":                    {},
	"device_key":                     {},
	"proposal_id":                    {},
	"approval_id":                    {},
	"prepared_audit_id":              {},
	"execution_id":                   {},
	"policy_decision_id":             {},
	"failure_class":                  {},
	"provider_class":                 {},
	"operator_mode":                  {},
	"repair_outcome":                 {},
	"notification_key":               {},
	"request_key":                    {},
	"export_id":                      {},
	"record_count":                   {},
	"source_ref":                     {},
	"event_type":                     {},
	"aggregate_version":              {},
	"partition_key":                  {},
	"operation_id":                   {},
	"operation_type":                 {},
	"target_ref_hash":                {},
	"risk_level":                     {},
	"status":                         {},
	"requested_by_hash":              {},
	"approved_by_hash":               {},
	"payload_hash":                   {},
	"payload_schema_version":         {},
	"reason_ref":                     {},
	"decision":                       {},
	"result_id":                      {},
	"downstream_service":             {},
	"downstream_request_ref":         {},
	"compensation_requested_by_hash": {},
	"compensation_reason_ref":        {},
}

type AdminEventMessage struct {
	Topic     string
	Partition int
	Offset    int64
	Key       []byte
	Value     []byte
}

type AuditRecord struct {
	TenantID           TenantID
	AuditID            string
	AuditStream        string
	SourceService      string
	SourceEventID      string
	RecordType         string
	ActorRef           string
	SubjectRef         string
	ResourceRef        string
	Action             string
	Outcome            string
	ReasonCode         string
	RiskLevel          string
	OccurredAt         time.Time
	IngestedAt         time.Time
	AttributesJSON     string
	CanonicalJSONHash  string
	PreviousRecordHash string
	RecordHash         string
	IdempotencyKey     string
	CommandHash        string
	CorrelationID      string
	CausationID        string
	TraceID            string
}

type AppendAuditRecordCommand struct {
	AuthContext    AuthContext
	AuditStream    string
	SourceService  string
	SourceEventID  string
	RecordType     string
	ActorRef       string
	SubjectRef     string
	ResourceRef    string
	Action         string
	Outcome        string
	ReasonCode     string
	RiskLevel      string
	OccurredAt     time.Time
	AttributesJSON string
	IdempotencyKey string
	CorrelationID  string
	CausationID    string
	TraceID        string
}

func (command AppendAuditRecordCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.SourceService) == "" {
		return NewInvalidArgument("source_service is required")
	}
	if strings.TrimSpace(command.SourceEventID) == "" {
		return NewInvalidArgument("source_event_id is required")
	}
	if strings.TrimSpace(command.RecordType) == "" {
		return NewInvalidArgument("record_type is required")
	}
	if strings.TrimSpace(command.Action) == "" {
		return NewInvalidArgument("action is required")
	}
	if strings.TrimSpace(command.Outcome) == "" {
		return NewInvalidArgument("outcome is required")
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if command.OccurredAt.IsZero() {
		return NewInvalidArgument("occurred_at is required")
	}
	if len(command.EffectiveAttributesJSON()) > MaxAttributesBytes {
		return NewInvalidArgument("attributes_json exceeds maximum")
	}
	if err := ValidateAttributesJSON(command.EffectiveAttributesJSON()); err != nil {
		return err
	}
	return nil
}

func (command AppendAuditRecordCommand) Normalized() AppendAuditRecordCommand {
	command.AuditStream = strings.TrimSpace(command.AuditStream)
	if command.AuditStream == "" {
		command.AuditStream = DefaultAuditStream
	}
	command.SourceService = strings.TrimSpace(command.SourceService)
	command.SourceEventID = strings.TrimSpace(command.SourceEventID)
	command.RecordType = strings.TrimSpace(command.RecordType)
	command.ActorRef = strings.TrimSpace(command.ActorRef)
	command.SubjectRef = strings.TrimSpace(command.SubjectRef)
	command.ResourceRef = strings.TrimSpace(command.ResourceRef)
	command.Action = strings.TrimSpace(command.Action)
	command.Outcome = strings.TrimSpace(command.Outcome)
	command.ReasonCode = strings.TrimSpace(command.ReasonCode)
	command.RiskLevel = strings.TrimSpace(command.RiskLevel)
	command.OccurredAt = command.OccurredAt.UTC()
	command.AttributesJSON = command.EffectiveAttributesJSON()
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	command.TraceID = strings.TrimSpace(command.TraceID)
	if command.TraceID == "" {
		command.TraceID = strings.TrimSpace(command.AuthContext.TraceID)
	}
	return command
}

func (command AppendAuditRecordCommand) EffectiveAttributesJSON() string {
	value := strings.TrimSpace(command.AttributesJSON)
	if value == "" {
		return "{}"
	}
	return value
}

type QueryAuditRecordsCommand struct {
	AuthContext   AuthContext
	AuditStream   string
	RecordType    string
	SourceService string
	AfterAuditID  string
	Limit         int
}

func (command QueryAuditRecordsCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.Limit < 0 {
		return NewInvalidArgument("limit must be non-negative")
	}
	return nil
}

func (command QueryAuditRecordsCommand) EffectiveLimit() int {
	if command.Limit <= 0 {
		return DefaultLimit
	}
	if command.Limit > MaxLimit {
		return MaxLimit
	}
	return command.Limit
}

type QueryAuditRecordsResult struct {
	Records    []AuditRecord
	NextCursor string
}

type AuditExportJob struct {
	TenantID         TenantID
	ExportID         string
	Status           string
	AuditStream      string
	RecordType       string
	SourceService    string
	FilterHash       string
	RedactionProfile string
	RequestedByRef   string
	RequestedAt      time.Time
	ManifestRef      string
	RecordCount      int64
	CompletedAt      time.Time
	FailedAt         time.Time
	PublicError      string
	IdempotencyKey   string
	CommandHash      string
	CorrelationID    string
	CausationID      string
	TraceID          string
}

type CreateAuditExportCommand struct {
	AuthContext      AuthContext
	AuditStream      string
	RecordType       string
	SourceService    string
	FilterHash       string
	RedactionProfile string
	RequestedByRef   string
	IdempotencyKey   string
	CorrelationID    string
	CausationID      string
	TraceID          string
}

func (command CreateAuditExportCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.FilterHash) == "" {
		return NewInvalidArgument("filter_hash is required")
	}
	if strings.TrimSpace(command.RedactionProfile) == "" {
		return NewInvalidArgument("redaction_profile is required")
	}
	if strings.TrimSpace(command.RequestedByRef) == "" {
		return NewInvalidArgument("requested_by_ref is required")
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	return nil
}

func (command CreateAuditExportCommand) Normalized() CreateAuditExportCommand {
	command.AuditStream = strings.TrimSpace(command.AuditStream)
	if command.AuditStream == "" {
		command.AuditStream = DefaultAuditStream
	}
	command.RecordType = strings.TrimSpace(command.RecordType)
	command.SourceService = strings.TrimSpace(command.SourceService)
	command.FilterHash = strings.TrimSpace(command.FilterHash)
	command.RedactionProfile = strings.TrimSpace(command.RedactionProfile)
	command.RequestedByRef = strings.TrimSpace(command.RequestedByRef)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	command.TraceID = strings.TrimSpace(command.TraceID)
	if command.TraceID == "" {
		command.TraceID = strings.TrimSpace(command.AuthContext.TraceID)
	}
	return command
}

type GetAuditExportCommand struct {
	AuthContext AuthContext
	ExportID    string
}

func (command GetAuditExportCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.ExportID) == "" {
		return NewInvalidArgument("export_id is required")
	}
	return nil
}

type VerifyAuditProofCommand struct {
	AuthContext AuthContext
	AuditID     string
}

func (command VerifyAuditProofCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.AuditID) == "" {
		return NewInvalidArgument("audit_id is required")
	}
	return nil
}

type AuditProofVerification struct {
	AuditID            string
	Valid              bool
	FailureReason      string
	RecordHash         string
	PreviousRecordHash string
}

func ValidateAttributesJSON(value string) error {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return NewInvalidArgument("attributes_json must be valid json")
	}
	if decoded == nil {
		return NewInvalidArgument("attributes_json must be a json object")
	}
	for key := range decoded {
		normalized := strings.TrimSpace(key)
		if normalized == "" {
			return NewInvalidArgument("attributes_json key is required")
		}
		if _, ok := allowedAttributeKeys[normalized]; !ok {
			return NewInvalidArgument("attributes_json contains disallowed key")
		}
	}
	return nil
}
