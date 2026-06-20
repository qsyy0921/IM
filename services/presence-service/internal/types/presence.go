package types

import (
	"strings"
	"time"
)

const (
	PresenceStateOffline      = "OFFLINE"
	PresenceStateOnline       = "ONLINE"
	PresenceStateAway         = "AWAY"
	PresenceStateDoNotDisturb = "DO_NOT_DISTURB"
	PresenceStateInvisible    = "INVISIBLE"
	PresenceStateUnknown      = "UNKNOWN"

	DeviceStateConnected       = "CONNECTED"
	DeviceStateHeartbeatActive = "HEARTBEAT_ACTIVE"
	DeviceStateStale           = "STALE"
	DeviceStateDisconnected    = "DISCONNECTED"
	DeviceStateRevoked         = "REVOKED"

	SourcePushGateway = "PUSH_GATEWAY"
	SourceClient      = "CLIENT"
	SourceOperator    = "OPERATOR"

	TypingStateStarted = "STARTED"
	TypingStateStopped = "STOPPED"

	VisibilityAllowed      = "ALLOWED"
	VisibilitySelf         = "SELF"
	VisibilityService      = "SERVICE"
	VisibilityDenied       = "DENIED"
	VisibilityInvisible    = "INVISIBLE"
	VisibilityUnavailable  = "UNAVAILABLE"
	VisibilityNotAvailable = "NOT_AVAILABLE"

	DefaultPresenceTTL = 5 * time.Minute
	MaxPresenceTTL     = 24 * time.Hour
	DefaultTypingTTL   = 15 * time.Second
	MaxTypingTTL       = 5 * time.Minute
)

var validPresenceStates = map[string]struct{}{
	PresenceStateOffline:      {},
	PresenceStateOnline:       {},
	PresenceStateAway:         {},
	PresenceStateDoNotDisturb: {},
	PresenceStateInvisible:    {},
}

var validSources = map[string]struct{}{
	SourcePushGateway: {},
	SourceClient:      {},
	SourceOperator:    {},
}

var validTypingStates = map[string]struct{}{
	TypingStateStarted: {},
	TypingStateStopped: {},
}

type PresenceState struct {
	TenantID           TenantID
	UserID             string
	ActualState        string
	VisibleState       string
	ManualStatus       string
	LastSeenAt         time.Time
	DeviceCount        int
	DeviceStates       []DevicePresence
	VisibilityDecision string
}

type DevicePresence struct {
	DeviceID    string
	SessionID   string
	State       string
	DeviceState string
	LastSeenAt  time.Time
	ExpiresAt   time.Time
}

type TypingIndicator struct {
	TenantID       TenantID
	ConversationID string
	UserID         string
	DeviceID       string
	TypingState    string
	ExpiresAt      time.Time
}

type UpdatePresenceCommand struct {
	AuthContext    AuthContext
	UserID         string
	DeviceID       string
	SessionID      string
	PresenceState  string
	ManualStatus   string
	TTL            time.Duration
	Source         string
	IdempotencyKey string
	CorrelationID  string
	CausationID    string
	TraceID        string
}

func (command UpdatePresenceCommand) Validate() error {
	if err := command.AuthContext.ValidateTenant(); err != nil {
		return err
	}
	if strings.TrimSpace(command.UserID) == "" {
		return NewInvalidArgument("user_id is required")
	}
	if !command.AuthContext.IsService() && strings.TrimSpace(command.AuthContext.UserID) != "" &&
		strings.TrimSpace(command.AuthContext.UserID) != strings.TrimSpace(command.UserID) {
		return NewPermissionDenied("user_id must match verified auth")
	}
	if strings.TrimSpace(command.DeviceID) == "" {
		return NewInvalidArgument("device_id is required")
	}
	if strings.TrimSpace(command.SessionID) == "" {
		return NewInvalidArgument("session_id is required")
	}
	if !IsValidPresenceState(command.PresenceState) {
		return NewInvalidArgument("presence_state is invalid")
	}
	if !IsValidSource(command.Source) {
		return NewInvalidArgument("source is invalid")
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if command.TTL <= 0 || command.TTL > MaxPresenceTTL {
		return NewInvalidArgument("ttl_ms is invalid")
	}
	return ValidateManualStatus(command.ManualStatus)
}

func (command UpdatePresenceCommand) Normalized() UpdatePresenceCommand {
	command.UserID = strings.TrimSpace(command.UserID)
	command.DeviceID = strings.TrimSpace(command.DeviceID)
	command.SessionID = strings.TrimSpace(command.SessionID)
	command.PresenceState = strings.ToUpper(strings.TrimSpace(command.PresenceState))
	command.ManualStatus = strings.TrimSpace(command.ManualStatus)
	command.Source = strings.ToUpper(strings.TrimSpace(command.Source))
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	command.TraceID = strings.TrimSpace(command.TraceID)
	if command.TraceID == "" {
		command.TraceID = strings.TrimSpace(command.AuthContext.TraceID)
	}
	return command
}

type GetPresenceCommand struct {
	AuthContext     AuthContext
	RequesterUserID string
	TargetUserIDs   []string
	ConversationID  string
	IncludeDevices  bool
}

func (command GetPresenceCommand) Validate() error {
	if err := command.AuthContext.ValidateTenant(); err != nil {
		return err
	}
	if strings.TrimSpace(command.RequesterUserID) == "" && !command.AuthContext.IsService() {
		return NewInvalidArgument("requester_user_id is required")
	}
	if !command.AuthContext.IsService() && strings.TrimSpace(command.AuthContext.UserID) != "" &&
		strings.TrimSpace(command.AuthContext.UserID) != strings.TrimSpace(command.RequesterUserID) {
		return NewPermissionDenied("requester_user_id must match verified auth")
	}
	if len(command.TargetUserIDs) == 0 {
		return NewInvalidArgument("target_user_ids is required")
	}
	if len(command.TargetUserIDs) > 100 {
		return NewInvalidArgument("target_user_ids exceeds maximum")
	}
	for _, target := range command.TargetUserIDs {
		if strings.TrimSpace(target) == "" {
			return NewInvalidArgument("target_user_id is required")
		}
	}
	return nil
}

func (command GetPresenceCommand) Normalized() GetPresenceCommand {
	command.RequesterUserID = strings.TrimSpace(command.RequesterUserID)
	command.ConversationID = strings.TrimSpace(command.ConversationID)
	targets := make([]string, 0, len(command.TargetUserIDs))
	seen := map[string]struct{}{}
	for _, target := range command.TargetUserIDs {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		targets = append(targets, target)
	}
	command.TargetUserIDs = targets
	return command
}

type UpdateTypingCommand struct {
	AuthContext    AuthContext
	ConversationID string
	UserID         string
	DeviceID       string
	TypingState    string
	TTL            time.Duration
	CorrelationID  string
	CausationID    string
	TraceID        string
}

func (command UpdateTypingCommand) Validate() error {
	if err := command.AuthContext.ValidateTenant(); err != nil {
		return err
	}
	if strings.TrimSpace(command.ConversationID) == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if strings.TrimSpace(command.UserID) == "" {
		return NewInvalidArgument("user_id is required")
	}
	if !command.AuthContext.IsService() && strings.TrimSpace(command.AuthContext.UserID) != "" &&
		strings.TrimSpace(command.AuthContext.UserID) != strings.TrimSpace(command.UserID) {
		return NewPermissionDenied("user_id must match verified auth")
	}
	if strings.TrimSpace(command.DeviceID) == "" {
		return NewInvalidArgument("device_id is required")
	}
	if !IsValidTypingState(command.TypingState) {
		return NewInvalidArgument("typing_state is invalid")
	}
	if command.TTL <= 0 || command.TTL > MaxTypingTTL {
		return NewInvalidArgument("ttl_ms is invalid")
	}
	return nil
}

func (command UpdateTypingCommand) Normalized() UpdateTypingCommand {
	command.ConversationID = strings.TrimSpace(command.ConversationID)
	command.UserID = strings.TrimSpace(command.UserID)
	command.DeviceID = strings.TrimSpace(command.DeviceID)
	command.TypingState = strings.ToUpper(strings.TrimSpace(command.TypingState))
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	command.TraceID = strings.TrimSpace(command.TraceID)
	if command.TraceID == "" {
		command.TraceID = strings.TrimSpace(command.AuthContext.TraceID)
	}
	return command
}

func IsValidPresenceState(state string) bool {
	_, ok := validPresenceStates[strings.ToUpper(strings.TrimSpace(state))]
	return ok
}

func IsValidSource(source string) bool {
	_, ok := validSources[strings.ToUpper(strings.TrimSpace(source))]
	return ok
}

func IsValidTypingState(state string) bool {
	_, ok := validTypingStates[strings.ToUpper(strings.TrimSpace(state))]
	return ok
}

func VisibleStateForActual(actual string) string {
	switch strings.ToUpper(strings.TrimSpace(actual)) {
	case PresenceStateInvisible:
		return PresenceStateOffline
	case PresenceStateOnline, PresenceStateAway, PresenceStateDoNotDisturb, PresenceStateOffline:
		return strings.ToUpper(strings.TrimSpace(actual))
	default:
		return PresenceStateUnknown
	}
}

func ValidateManualStatus(value string) error {
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		return NewInvalidArgument("manual_status exceeds maximum")
	}
	normalized := strings.ToLower(value)
	for _, forbidden := range []string{"secret", "password", "token", "private_key", "dsn"} {
		if strings.Contains(normalized, forbidden) {
			return NewInvalidArgument("manual_status contains unsafe text")
		}
	}
	return nil
}
