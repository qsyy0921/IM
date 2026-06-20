package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

const (
	ChannelEmail  = "EMAIL"
	ChannelSMS    = "SMS"
	ChannelAPNS   = "APNS"
	ChannelFCM    = "FCM"
	ChannelSystem = "SYSTEM"

	PriorityLow    = "LOW"
	PriorityNormal = "NORMAL"
	PriorityHigh   = "HIGH"

	StatusAccepted  = "ACCEPTED"
	StatusScheduled = "SCHEDULED"
	StatusSending   = "SENDING"
	StatusRetryWait = "RETRY_WAIT"
	StatusDelivered = "DELIVERED"
	StatusDLQ       = "DLQ"
	StatusCanceled  = "CANCELED"

	DefaultLocale = "und"

	MaxTemplateVariablesBytes = 16 * 1024
	MaxSecretPayloadBytes     = 16 * 1024
)

type NotificationRequest struct {
	TenantID                TenantID
	RequestID               string
	RequesterService        string
	RequesterUserID         UserID
	IdempotencyKey          string
	CommandHash             string
	Channel                 string
	RecipientRef            string
	DestinationHash         string
	DestinationMasked       string
	TemplateKey             string
	TemplateVersion         string
	Locale                  string
	Priority                string
	TemplateVariablesJSON   string
	SecretPayloadCiphertext []byte
	SecretPayloadKeyVersion string
	SecretPayloadExpiresAt  time.Time
	Status                  string
	AttemptCount            int
	NextAttemptAt           time.Time
	ExpiresAt               time.Time
	LastFailureClass        string
	LastPublicError         string
	CorrelationID           string
	CausationID             string
	TraceID                 string
	CreatedAt               time.Time
	DeliveredAt             time.Time
	DeadLetteredAt          time.Time
	CanceledAt              time.Time
}

type CreateNotificationRequestCommand struct {
	AuthContext             AuthContext
	RequesterService        string
	RequesterUserID         UserID
	Channel                 string
	RecipientRef            string
	DestinationRef          string
	DestinationMasked       string
	TemplateKey             string
	TemplateVersion         string
	Locale                  string
	Priority                string
	ScheduledAt             time.Time
	ExpiresAt               time.Time
	IdempotencyKey          string
	TemplateVariablesJSON   string
	SecretPayloadCiphertext []byte
	SecretPayloadKeyVersion string
	SecretPayloadExpiresAt  time.Time
	CorrelationID           string
	CausationID             string
	TraceID                 string
}

func (command CreateNotificationRequestCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.RequesterService) == "" {
		return NewInvalidArgument("requester_service is required")
	}
	if !IsValidChannel(command.Channel) {
		return NewInvalidArgument("invalid channel")
	}
	if strings.TrimSpace(command.RecipientRef) == "" {
		return NewInvalidArgument("recipient_ref is required")
	}
	if strings.TrimSpace(command.DestinationRef) == "" {
		return NewInvalidArgument("destination_ref is required")
	}
	if strings.TrimSpace(command.TemplateKey) == "" {
		return NewInvalidArgument("template_key is required")
	}
	if strings.TrimSpace(command.TemplateVersion) == "" {
		return NewInvalidArgument("template_version is required")
	}
	if !IsValidPriority(command.EffectivePriority()) {
		return NewInvalidArgument("invalid priority")
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if len(command.TemplateVariablesJSON) > MaxTemplateVariablesBytes {
		return NewInvalidArgument("template_variables_json exceeds maximum")
	}
	if err := validateJSONObject(command.EffectiveTemplateVariablesJSON(), "template_variables_json"); err != nil {
		return err
	}
	if len(command.SecretPayloadCiphertext) > MaxSecretPayloadBytes {
		return NewInvalidArgument("secret_payload_ciphertext exceeds maximum")
	}
	if len(command.SecretPayloadCiphertext) > 0 && strings.TrimSpace(command.SecretPayloadKeyVersion) == "" {
		return NewInvalidArgument("secret_payload_key_version is required")
	}
	if len(command.SecretPayloadCiphertext) > 0 && command.SecretPayloadExpiresAt.IsZero() {
		return NewInvalidArgument("secret_payload_expires_at is required")
	}
	if !command.ExpiresAt.IsZero() && !command.ScheduledAt.IsZero() && command.ExpiresAt.Before(command.ScheduledAt) {
		return NewInvalidArgument("expires_at must be after scheduled_at")
	}
	return nil
}

func (command CreateNotificationRequestCommand) Normalized() CreateNotificationRequestCommand {
	command.RequesterService = strings.TrimSpace(command.RequesterService)
	if command.RequesterUserID == "" {
		command.RequesterUserID = command.AuthContext.UserID
	}
	command.Channel = strings.TrimSpace(command.Channel)
	command.RecipientRef = strings.TrimSpace(command.RecipientRef)
	command.DestinationRef = strings.TrimSpace(command.DestinationRef)
	command.DestinationMasked = strings.TrimSpace(command.DestinationMasked)
	command.TemplateKey = strings.TrimSpace(command.TemplateKey)
	command.TemplateVersion = strings.TrimSpace(command.TemplateVersion)
	command.Locale = command.EffectiveLocale()
	command.Priority = command.EffectivePriority()
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.TemplateVariablesJSON = command.EffectiveTemplateVariablesJSON()
	command.SecretPayloadKeyVersion = strings.TrimSpace(command.SecretPayloadKeyVersion)
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	command.TraceID = strings.TrimSpace(command.TraceID)
	if command.TraceID == "" {
		command.TraceID = strings.TrimSpace(command.AuthContext.TraceID)
	}
	return command
}

func (command CreateNotificationRequestCommand) EffectivePriority() string {
	priority := strings.TrimSpace(command.Priority)
	if priority == "" {
		return PriorityNormal
	}
	return priority
}

func (command CreateNotificationRequestCommand) EffectiveLocale() string {
	locale := strings.TrimSpace(command.Locale)
	if locale == "" {
		return DefaultLocale
	}
	return locale
}

func (command CreateNotificationRequestCommand) EffectiveTemplateVariablesJSON() string {
	value := strings.TrimSpace(command.TemplateVariablesJSON)
	if value == "" {
		return "{}"
	}
	return value
}

func (command CreateNotificationRequestCommand) CommandHash(destinationHash string) string {
	normalized := command.Normalized()
	parts := []string{
		string(normalized.AuthContext.TenantID),
		normalized.RequesterService,
		string(normalized.RequesterUserID),
		normalized.Channel,
		normalized.RecipientRef,
		destinationHash,
		normalized.DestinationMasked,
		normalized.TemplateKey,
		normalized.TemplateVersion,
		normalized.Locale,
		normalized.Priority,
		normalized.ScheduledAt.UTC().Format(time.RFC3339Nano),
		normalized.ExpiresAt.UTC().Format(time.RFC3339Nano),
		normalized.IdempotencyKey,
		normalized.TemplateVariablesJSON,
		normalized.SecretPayloadKeyVersion,
		normalized.SecretPayloadExpiresAt.UTC().Format(time.RFC3339Nano),
		normalized.CorrelationID,
		normalized.CausationID,
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

type GetNotificationStatusCommand struct {
	AuthContext AuthContext
	RequestID   string
}

func (command GetNotificationStatusCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.RequestID) == "" {
		return NewInvalidArgument("request_id is required")
	}
	return nil
}

type CancelNotificationRequestCommand struct {
	AuthContext     AuthContext
	RequestID       string
	CancelRequestID string
	Reason          string
}

func (command CancelNotificationRequestCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.RequestID) == "" {
		return NewInvalidArgument("request_id is required")
	}
	if strings.TrimSpace(command.CancelRequestID) == "" {
		return NewInvalidArgument("cancel_request_id is required")
	}
	return nil
}

func IsValidChannel(channel string) bool {
	switch strings.TrimSpace(channel) {
	case ChannelEmail, ChannelSMS, ChannelAPNS, ChannelFCM, ChannelSystem:
		return true
	default:
		return false
	}
}

func IsValidPriority(priority string) bool {
	switch strings.TrimSpace(priority) {
	case PriorityLow, PriorityNormal, PriorityHigh:
		return true
	default:
		return false
	}
}

func IsTerminalStatus(status string) bool {
	switch status {
	case StatusDelivered, StatusDLQ, StatusCanceled:
		return true
	default:
		return false
	}
}

func validateJSONObject(value string, field string) error {
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return NewInvalidArgument(field + " must be valid json")
	}
	if _, ok := decoded.(map[string]any); !ok {
		return NewInvalidArgument(field + " must be a json object")
	}
	return nil
}
