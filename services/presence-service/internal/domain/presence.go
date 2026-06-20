package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/presence-service/internal/types"
)

type PreparedPresenceUpdate struct {
	Command     types.UpdatePresenceCommand
	CommandHash string
	DeviceState string
	ExpiresAt   time.Time
	ObservedAt  time.Time
}

type PreparedTypingUpdate struct {
	Command   types.UpdateTypingCommand
	ExpiresAt time.Time
	UpdatedAt time.Time
}

func PreparePresenceUpdate(command types.UpdatePresenceCommand, now time.Time) (PreparedPresenceUpdate, error) {
	if command.TTL == 0 {
		command.TTL = types.DefaultPresenceTTL
	}
	if command.Source == "" {
		command.Source = types.SourceClient
	}
	if err := command.Validate(); err != nil {
		return PreparedPresenceUpdate{}, err
	}
	normalized := command.Normalized()
	observedAt := now.UTC()
	hash, err := commandHash(normalized)
	if err != nil {
		return PreparedPresenceUpdate{}, err
	}
	return PreparedPresenceUpdate{
		Command:     normalized,
		CommandHash: hash,
		DeviceState: deviceStateForPresence(normalized.PresenceState),
		ExpiresAt:   observedAt.Add(normalized.TTL),
		ObservedAt:  observedAt,
	}, nil
}

func PrepareTypingUpdate(command types.UpdateTypingCommand, now time.Time) (PreparedTypingUpdate, error) {
	if command.TTL == 0 {
		command.TTL = types.DefaultTypingTTL
	}
	if err := command.Validate(); err != nil {
		return PreparedTypingUpdate{}, err
	}
	normalized := command.Normalized()
	updatedAt := now.UTC()
	expiresAt := updatedAt.Add(normalized.TTL)
	if normalized.TypingState == types.TypingStateStopped {
		expiresAt = updatedAt
	}
	return PreparedTypingUpdate{
		Command:   normalized,
		ExpiresAt: expiresAt,
		UpdatedAt: updatedAt,
	}, nil
}

func ApplyVisibility(command types.GetPresenceCommand, states []types.PresenceState) []types.PresenceState {
	normalized := command.Normalized()
	filtered := make([]types.PresenceState, 0, len(states))
	for _, state := range states {
		state.DeviceStates = cloneDevices(state.DeviceStates)
		switch {
		case normalized.AuthContext.IsService():
			state.VisibilityDecision = types.VisibilityService
			state.VisibleState = types.VisibleStateForActual(state.ActualState)
		case normalized.RequesterUserID == state.UserID:
			state.VisibilityDecision = types.VisibilitySelf
			state.VisibleState = types.VisibleStateForActual(state.ActualState)
		case state.ActualState == types.PresenceStateInvisible:
			state.VisibilityDecision = types.VisibilityInvisible
			state.VisibleState = types.PresenceStateOffline
			state.DeviceStates = nil
		default:
			state.VisibilityDecision = types.VisibilityDenied
			state.VisibleState = types.PresenceStateUnknown
			state.ManualStatus = ""
			state.DeviceCount = 0
			state.DeviceStates = nil
		}
		if !normalized.IncludeDevices {
			state.DeviceStates = nil
		}
		filtered = append(filtered, state)
	}
	return filtered
}

func HashRef(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func commandHash(command types.UpdatePresenceCommand) (string, error) {
	payload := map[string]any{
		"tenant_id":       string(command.AuthContext.TenantID),
		"user_id":         command.UserID,
		"device_id":       command.DeviceID,
		"session_id":      command.SessionID,
		"presence_state":  command.PresenceState,
		"manual_status":   command.ManualStatus,
		"ttl_ms":          command.TTL.Milliseconds(),
		"source":          command.Source,
		"idempotency_key": command.IdempotencyKey,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewInvalidArgument("presence command hash payload invalid")
	}
	return HashRef(string(encoded)), nil
}

func deviceStateForPresence(state string) string {
	if state == types.PresenceStateOffline {
		return types.DeviceStateDisconnected
	}
	return types.DeviceStateHeartbeatActive
}

func cloneDevices(devices []types.DevicePresence) []types.DevicePresence {
	if len(devices) == 0 {
		return nil
	}
	cloned := make([]types.DevicePresence, len(devices))
	copy(cloned, devices)
	return cloned
}
