package types

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	ConfigKindAPIGatewayTenantQuota     = "API_GATEWAY_TENANT_QUOTA"
	ConfigKindFeatureFlag               = "FEATURE_FLAG"
	ConfigKindPolicyRulesetRef          = "POLICY_RULESET_REF"
	ConfigKindPolicyTenantQuota         = "POLICY_TENANT_QUOTA"
	ConfigKindNotificationChannelPolicy = "NOTIFICATION_CHANNEL_POLICY"
	ConfigKindModelProviderPolicy       = "MODEL_PROVIDER_POLICY"
	ConfigKindMediaPolicy               = "MEDIA_POLICY"

	StatusActive    = "ACTIVE"
	StatusPublished = "PUBLISHED"

	AppliedStatusInSync          = "IN_SYNC"
	AppliedStatusStaleVersion    = "STALE_VERSION"
	AppliedStatusMissingAck      = "MISSING_ACK"
	AppliedStatusApplyFailed     = "APPLY_FAILED"
	AppliedStatusUnknownInstance = "UNKNOWN_INSTANCE"

	MaxPayloadBytes = 64 * 1024
)

var validConfigKinds = map[string]struct{}{
	ConfigKindAPIGatewayTenantQuota:     {},
	ConfigKindFeatureFlag:               {},
	ConfigKindPolicyRulesetRef:          {},
	ConfigKindPolicyTenantQuota:         {},
	ConfigKindNotificationChannelPolicy: {},
	ConfigKindModelProviderPolicy:       {},
	ConfigKindMediaPolicy:               {},
}

var validAppliedStatuses = map[string]struct{}{
	AppliedStatusInSync:          {},
	AppliedStatusStaleVersion:    {},
	AppliedStatusMissingAck:      {},
	AppliedStatusApplyFailed:     {},
	AppliedStatusUnknownInstance: {},
}

type ConfigVersion struct {
	TenantID        TenantID
	Environment     string
	ConfigKind      string
	BundleKey       string
	Version         string
	SchemaVersion   string
	PayloadJSON     string
	PayloadChecksum string
	CommandHash     string
	Status          string
	EffectiveAt     time.Time
	ExpiresAt       time.Time
	PublishedAt     time.Time
	ApprovalRef     string
	OperatorRef     string
	ReasonRef       string
	IdempotencyKey  string
	CorrelationID   string
	CausationID     string
	TraceID         string
}

type ConfigSnapshot struct {
	TenantID        TenantID
	Environment     string
	ServiceName     string
	ConfigKind      string
	BundleKey       string
	Version         string
	SchemaVersion   string
	PayloadJSON     string
	PayloadChecksum string
	GeneratedAt     time.Time
	EffectiveAt     time.Time
	ExpiresAt       time.Time
	RolloutDecision string
	PreviousVersion string
	NotModified     bool
}

type AppliedConfigVersion struct {
	TenantID       TenantID
	Environment    string
	ServiceName    string
	InstanceRef    string
	ConfigKind     string
	BundleKey      string
	Version        string
	ServiceVersion string
	Status         string
	LastErrorClass string
	AppliedAt      time.Time
}

type PublishConfigVersionCommand struct {
	AuthContext    AuthContext
	Environment    string
	ConfigKind     string
	BundleKey      string
	Version        string
	SchemaVersion  string
	PayloadJSON    string
	EffectiveAt    time.Time
	ExpiresAt      time.Time
	ApprovalRef    string
	OperatorRef    string
	ReasonRef      string
	IdempotencyKey string
	CorrelationID  string
	CausationID    string
	TraceID        string
}

func (command PublishConfigVersionCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.Environment) == "" {
		return NewInvalidArgument("environment is required")
	}
	if !IsValidConfigKind(command.ConfigKind) {
		return NewInvalidArgument("config_kind is invalid")
	}
	if strings.TrimSpace(command.BundleKey) == "" {
		return NewInvalidArgument("bundle_key is required")
	}
	if strings.TrimSpace(command.Version) == "" {
		return NewInvalidArgument("version is required")
	}
	if strings.TrimSpace(command.SchemaVersion) == "" {
		return NewInvalidArgument("schema_version is required")
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if command.EffectiveAt.IsZero() {
		return NewInvalidArgument("effective_at is required")
	}
	if !command.ExpiresAt.IsZero() && !command.ExpiresAt.After(command.EffectiveAt) {
		return NewInvalidArgument("expires_at must be after effective_at")
	}
	return ValidatePayloadJSON(command.PayloadJSON)
}

func (command PublishConfigVersionCommand) Normalized() PublishConfigVersionCommand {
	command.Environment = strings.TrimSpace(command.Environment)
	command.ConfigKind = strings.TrimSpace(command.ConfigKind)
	command.BundleKey = strings.TrimSpace(command.BundleKey)
	command.Version = strings.TrimSpace(command.Version)
	command.SchemaVersion = strings.TrimSpace(command.SchemaVersion)
	command.PayloadJSON = strings.TrimSpace(command.PayloadJSON)
	command.EffectiveAt = command.EffectiveAt.UTC()
	command.ExpiresAt = command.ExpiresAt.UTC()
	command.ApprovalRef = strings.TrimSpace(command.ApprovalRef)
	command.OperatorRef = strings.TrimSpace(command.OperatorRef)
	command.ReasonRef = strings.TrimSpace(command.ReasonRef)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	command.TraceID = strings.TrimSpace(command.TraceID)
	if command.TraceID == "" {
		command.TraceID = strings.TrimSpace(command.AuthContext.TraceID)
	}
	return command
}

type GetConfigSnapshotCommand struct {
	AuthContext    AuthContext
	Environment    string
	ServiceName    string
	ConfigKind     string
	BundleKey      string
	CurrentVersion string
	InstanceRef    string
	Ring           string
	ServiceVersion string
}

func (command GetConfigSnapshotCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.Environment) == "" {
		return NewInvalidArgument("environment is required")
	}
	if strings.TrimSpace(command.ServiceName) == "" {
		return NewInvalidArgument("service_name is required")
	}
	if !IsValidConfigKind(command.ConfigKind) {
		return NewInvalidArgument("config_kind is invalid")
	}
	if strings.TrimSpace(command.BundleKey) == "" {
		return NewInvalidArgument("bundle_key is required")
	}
	return nil
}

func (command GetConfigSnapshotCommand) Normalized() GetConfigSnapshotCommand {
	command.Environment = strings.TrimSpace(command.Environment)
	command.ServiceName = strings.TrimSpace(command.ServiceName)
	command.ConfigKind = strings.TrimSpace(command.ConfigKind)
	command.BundleKey = strings.TrimSpace(command.BundleKey)
	command.CurrentVersion = strings.TrimSpace(command.CurrentVersion)
	command.InstanceRef = strings.TrimSpace(command.InstanceRef)
	command.Ring = strings.TrimSpace(command.Ring)
	command.ServiceVersion = strings.TrimSpace(command.ServiceVersion)
	return command
}

type AckAppliedConfigVersionCommand struct {
	AuthContext    AuthContext
	Environment    string
	ServiceName    string
	InstanceRef    string
	ConfigKind     string
	BundleKey      string
	Version        string
	ServiceVersion string
	Status         string
	LastErrorClass string
	CorrelationID  string
	CausationID    string
	TraceID        string
}

func (command AckAppliedConfigVersionCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.Environment) == "" {
		return NewInvalidArgument("environment is required")
	}
	if strings.TrimSpace(command.ServiceName) == "" {
		return NewInvalidArgument("service_name is required")
	}
	if strings.TrimSpace(command.InstanceRef) == "" {
		return NewInvalidArgument("instance_ref is required")
	}
	if !IsValidConfigKind(command.ConfigKind) {
		return NewInvalidArgument("config_kind is invalid")
	}
	if strings.TrimSpace(command.BundleKey) == "" {
		return NewInvalidArgument("bundle_key is required")
	}
	if strings.TrimSpace(command.Version) == "" {
		return NewInvalidArgument("version is required")
	}
	if _, ok := validAppliedStatuses[strings.TrimSpace(command.Status)]; !ok {
		return NewInvalidArgument("status is invalid")
	}
	return nil
}

func (command AckAppliedConfigVersionCommand) Normalized() AckAppliedConfigVersionCommand {
	command.Environment = strings.TrimSpace(command.Environment)
	command.ServiceName = strings.TrimSpace(command.ServiceName)
	command.InstanceRef = strings.TrimSpace(command.InstanceRef)
	command.ConfigKind = strings.TrimSpace(command.ConfigKind)
	command.BundleKey = strings.TrimSpace(command.BundleKey)
	command.Version = strings.TrimSpace(command.Version)
	command.ServiceVersion = strings.TrimSpace(command.ServiceVersion)
	command.Status = strings.TrimSpace(command.Status)
	command.LastErrorClass = strings.TrimSpace(command.LastErrorClass)
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	command.TraceID = strings.TrimSpace(command.TraceID)
	if command.TraceID == "" {
		command.TraceID = strings.TrimSpace(command.AuthContext.TraceID)
	}
	return command
}

func IsValidConfigKind(kind string) bool {
	_, ok := validConfigKinds[strings.TrimSpace(kind)]
	return ok
}

func ValidatePayloadJSON(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return NewInvalidArgument("payload_json is required")
	}
	if len(value) > MaxPayloadBytes {
		return NewInvalidArgument("payload_json exceeds maximum")
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return NewInvalidArgument("payload_json must be valid json")
	}
	if decoded == nil {
		return NewInvalidArgument("payload_json must be a json object")
	}
	for key := range decoded {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "" {
			return NewInvalidArgument("payload_json key is required")
		}
		if strings.Contains(normalized, "secret") ||
			strings.Contains(normalized, "password") ||
			strings.Contains(normalized, "token") ||
			strings.Contains(normalized, "private_key") ||
			strings.Contains(normalized, "dsn") {
			return NewInvalidArgument("payload_json contains unsafe key")
		}
	}
	return nil
}
