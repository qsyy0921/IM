package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/presence-service/internal/domain"
	"github.com/qsyy0921/IM/services/presence-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) UpdatePresence(
	ctx context.Context,
	prepared domain.PreparedPresenceUpdate,
	eventID string,
) (types.PresenceState, error) {
	if repository.pool == nil {
		return types.PresenceState{}, types.NewDBWriteFailed("presence repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.PresenceState{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	replayed, err := checkIdempotency(ctx, tx, prepared)
	if err != nil {
		return types.PresenceState{}, err
	}
	if replayed {
		state, err := loadPresenceStateTx(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.UserID, true)
		if err != nil {
			return types.PresenceState{}, types.NewDBReadFailed(err.Error())
		}
		if err := tx.Commit(ctx); err != nil {
			return types.PresenceState{}, types.NewDBWriteFailed(err.Error())
		}
		return state, nil
	}
	if err := upsertSession(ctx, tx, prepared); err != nil {
		return types.PresenceState{}, types.NewDBWriteFailed(err.Error())
	}
	state, err := recomputeUserState(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.UserID)
	if err != nil {
		return types.PresenceState{}, err
	}
	if err := insertPresenceOutbox(ctx, tx, eventID, state, "presence.user.changed.v1"); err != nil {
		return types.PresenceState{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.PresenceState{}, types.NewDBWriteFailed(err.Error())
	}
	return state, nil
}

func (repository *Repository) GetPresenceStates(
	ctx context.Context,
	command types.GetPresenceCommand,
) ([]types.PresenceState, error) {
	if repository.pool == nil {
		return nil, types.NewDBReadFailed("presence repository is not configured")
	}
	states := make([]types.PresenceState, 0, len(command.TargetUserIDs))
	for _, target := range command.TargetUserIDs {
		state, err := repository.loadPresenceState(ctx, command.AuthContext.TenantID, target, command.IncludeDevices)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

func (repository *Repository) UpdateTyping(
	ctx context.Context,
	prepared domain.PreparedTypingUpdate,
	eventID string,
) (types.TypingIndicator, error) {
	if repository.pool == nil {
		return types.TypingIndicator{}, types.NewDBWriteFailed("presence repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.TypingIndicator{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	command := prepared.Command
	typing := types.TypingIndicator{
		TenantID:       command.AuthContext.TenantID,
		ConversationID: command.ConversationID,
		UserID:         command.UserID,
		DeviceID:       command.DeviceID,
		TypingState:    command.TypingState,
		ExpiresAt:      prepared.ExpiresAt,
	}
	if err := upsertTyping(ctx, tx, typing); err != nil {
		return types.TypingIndicator{}, types.NewDBWriteFailed(err.Error())
	}
	if err := insertTypingOutbox(ctx, tx, eventID, typing); err != nil {
		return types.TypingIndicator{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.TypingIndicator{}, types.NewDBWriteFailed(err.Error())
	}
	return typing, nil
}

func checkIdempotency(ctx context.Context, tx pgx.Tx, prepared domain.PreparedPresenceUpdate) (bool, error) {
	command := prepared.Command
	row := tx.QueryRow(ctx, `
SELECT last_command_hash
FROM presence_sessions
WHERE tenant_id = $1
  AND user_id = $2
  AND last_idempotency_key = $3
LIMIT 1
`, string(command.AuthContext.TenantID), command.UserID, command.IdempotencyKey)
	var commandHash string
	if err := row.Scan(&commandHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, types.NewDBReadFailed(err.Error())
	}
	if commandHash != prepared.CommandHash {
		return false, types.NewAlreadyExists("presence update idempotency conflict")
	}
	return true, nil
}

func upsertSession(ctx context.Context, tx pgx.Tx, prepared domain.PreparedPresenceUpdate) error {
	command := prepared.Command
	_, err := tx.Exec(ctx, `
INSERT INTO presence_sessions (
    tenant_id, session_id, user_id, device_id, presence_state, device_state,
    source, manual_status, last_seen_at, expires_at, last_idempotency_key,
    last_command_hash, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, now(), now()
)
ON CONFLICT (tenant_id, session_id)
DO UPDATE SET
    user_id = EXCLUDED.user_id,
    device_id = EXCLUDED.device_id,
    presence_state = EXCLUDED.presence_state,
    device_state = EXCLUDED.device_state,
    source = EXCLUDED.source,
    manual_status = EXCLUDED.manual_status,
    last_seen_at = GREATEST(presence_sessions.last_seen_at, EXCLUDED.last_seen_at),
    expires_at = EXCLUDED.expires_at,
    last_idempotency_key = EXCLUDED.last_idempotency_key,
    last_command_hash = EXCLUDED.last_command_hash,
    updated_at = now()
`, string(command.AuthContext.TenantID), command.SessionID, command.UserID, command.DeviceID,
		command.PresenceState, prepared.DeviceState, command.Source, command.ManualStatus,
		prepared.ObservedAt, prepared.ExpiresAt, command.IdempotencyKey, prepared.CommandHash)
	return err
}

func recomputeUserState(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID string) (types.PresenceState, error) {
	now := time.Now().UTC()
	var deviceCount int
	if err := tx.QueryRow(ctx, `
SELECT count(*)
FROM presence_sessions
WHERE tenant_id = $1
  AND user_id = $2
  AND expires_at > $3
  AND device_state IN ('CONNECTED', 'HEARTBEAT_ACTIVE')
  AND presence_state <> 'OFFLINE'
`, string(tenantID), userID, now).Scan(&deviceCount); err != nil {
		return types.PresenceState{}, err
	}

	actualState := types.PresenceStateOffline
	manualStatus := ""
	lastSeenAt := now
	if deviceCount > 0 {
		row := tx.QueryRow(ctx, `
SELECT presence_state, manual_status, last_seen_at
FROM presence_sessions
WHERE tenant_id = $1
  AND user_id = $2
  AND expires_at > $3
  AND device_state IN ('CONNECTED', 'HEARTBEAT_ACTIVE')
  AND presence_state <> 'OFFLINE'
ORDER BY updated_at DESC, last_seen_at DESC
LIMIT 1
`, string(tenantID), userID, now)
		if err := row.Scan(&actualState, &manualStatus, &lastSeenAt); err != nil {
			return types.PresenceState{}, err
		}
	} else {
		_ = tx.QueryRow(ctx, `
SELECT COALESCE(max(last_seen_at), $3)
FROM presence_sessions
WHERE tenant_id = $1
  AND user_id = $2
`, string(tenantID), userID, now).Scan(&lastSeenAt)
	}

	visibleState := types.VisibleStateForActual(actualState)
	_, err := tx.Exec(ctx, `
INSERT INTO presence_user_states (
    tenant_id, user_id, actual_state, visible_state, manual_status,
    last_seen_at, device_count, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (tenant_id, user_id)
DO UPDATE SET
    actual_state = EXCLUDED.actual_state,
    visible_state = EXCLUDED.visible_state,
    manual_status = EXCLUDED.manual_status,
    last_seen_at = GREATEST(presence_user_states.last_seen_at, EXCLUDED.last_seen_at),
    device_count = EXCLUDED.device_count,
    updated_at = now()
`, string(tenantID), userID, actualState, visibleState, manualStatus, lastSeenAt, deviceCount)
	if err != nil {
		return types.PresenceState{}, err
	}
	return loadPresenceStateTx(ctx, tx, tenantID, userID, true)
}

func (repository *Repository) loadPresenceState(
	ctx context.Context,
	tenantID types.TenantID,
	userID string,
	includeDevices bool,
) (types.PresenceState, error) {
	row := repository.pool.QueryRow(ctx, `
SELECT tenant_id, user_id, actual_state, visible_state, manual_status,
       last_seen_at, device_count
FROM presence_user_states
WHERE tenant_id = $1
  AND user_id = $2
`, string(tenantID), userID)
	state, err := scanPresenceState(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.PresenceState{
				TenantID:           tenantID,
				UserID:             userID,
				ActualState:        types.PresenceStateOffline,
				VisibleState:       types.PresenceStateOffline,
				VisibilityDecision: types.VisibilityNotAvailable,
			}, nil
		}
		return types.PresenceState{}, types.NewDBReadFailed(err.Error())
	}
	if includeDevices {
		devices, err := repository.loadDevices(ctx, tenantID, userID)
		if err != nil {
			return types.PresenceState{}, err
		}
		state.DeviceStates = devices
	}
	return state, nil
}

func loadPresenceStateTx(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID string,
	includeDevices bool,
) (types.PresenceState, error) {
	row := tx.QueryRow(ctx, `
SELECT tenant_id, user_id, actual_state, visible_state, manual_status,
       last_seen_at, device_count
FROM presence_user_states
WHERE tenant_id = $1
  AND user_id = $2
`, string(tenantID), userID)
	state, err := scanPresenceState(row)
	if err != nil {
		return types.PresenceState{}, err
	}
	if includeDevices {
		devices, err := loadDevicesTx(ctx, tx, tenantID, userID)
		if err != nil {
			return types.PresenceState{}, err
		}
		state.DeviceStates = devices
	}
	return state, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPresenceState(row rowScanner) (types.PresenceState, error) {
	var state types.PresenceState
	if err := row.Scan(
		&state.TenantID,
		&state.UserID,
		&state.ActualState,
		&state.VisibleState,
		&state.ManualStatus,
		&state.LastSeenAt,
		&state.DeviceCount,
	); err != nil {
		return types.PresenceState{}, err
	}
	return state, nil
}

func (repository *Repository) loadDevices(
	ctx context.Context,
	tenantID types.TenantID,
	userID string,
) ([]types.DevicePresence, error) {
	rows, err := repository.pool.Query(ctx, `
SELECT device_id, session_id, presence_state, device_state, last_seen_at, expires_at
FROM presence_sessions
WHERE tenant_id = $1
  AND user_id = $2
ORDER BY updated_at DESC
`, string(tenantID), userID)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()
	return scanDevices(rows)
}

func loadDevicesTx(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID string) ([]types.DevicePresence, error) {
	rows, err := tx.Query(ctx, `
SELECT device_id, session_id, presence_state, device_state, last_seen_at, expires_at
FROM presence_sessions
WHERE tenant_id = $1
  AND user_id = $2
ORDER BY updated_at DESC
`, string(tenantID), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDevices(rows)
}

func scanDevices(rows pgx.Rows) ([]types.DevicePresence, error) {
	var devices []types.DevicePresence
	for rows.Next() {
		var device types.DevicePresence
		if err := rows.Scan(
			&device.DeviceID,
			&device.SessionID,
			&device.State,
			&device.DeviceState,
			&device.LastSeenAt,
			&device.ExpiresAt,
		); err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func upsertTyping(ctx context.Context, tx pgx.Tx, typing types.TypingIndicator) error {
	_, err := tx.Exec(ctx, `
INSERT INTO presence_typing_indicators (
    tenant_id, conversation_id, user_id, device_id, typing_state, expires_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (tenant_id, conversation_id, user_id, device_id)
DO UPDATE SET
    typing_state = EXCLUDED.typing_state,
    expires_at = EXCLUDED.expires_at,
    updated_at = now()
`, string(typing.TenantID), typing.ConversationID, typing.UserID, typing.DeviceID, typing.TypingState, typing.ExpiresAt)
	return err
}

func insertPresenceOutbox(ctx context.Context, tx pgx.Tx, eventID string, state types.PresenceState, eventType string) error {
	payload := map[string]any{
		"tenant_ref":   domain.HashRef(string(state.TenantID)),
		"user_ref":     domain.HashRef(state.UserID),
		"state":        state.VisibleState,
		"actual_state": state.ActualState,
		"device_count": state.DeviceCount,
		"last_seen_at": state.LastSeenAt.UTC().UnixMilli(),
	}
	return insertOutbox(ctx, tx, eventID, state.TenantID, "presence_user",
		domain.HashRef(string(state.TenantID)+":"+state.UserID), eventType,
		domain.HashRef(string(state.TenantID)+":"+state.UserID), payload)
}

func insertTypingOutbox(ctx context.Context, tx pgx.Tx, eventID string, typing types.TypingIndicator) error {
	payload := map[string]any{
		"tenant_ref":       domain.HashRef(string(typing.TenantID)),
		"conversation_ref": domain.HashRef(typing.ConversationID),
		"user_ref":         domain.HashRef(typing.UserID),
		"device_ref":       domain.HashRef(typing.DeviceID),
		"typing_state":     typing.TypingState,
		"expires_at":       typing.ExpiresAt.UTC().UnixMilli(),
	}
	return insertOutbox(ctx, tx, eventID, typing.TenantID, "typing_indicator",
		domain.HashRef(string(typing.TenantID)+":"+typing.ConversationID+":"+typing.UserID),
		"presence.typing.changed.v1", domain.HashRef(string(typing.TenantID)+":"+typing.ConversationID), payload)
}

func insertOutbox(
	ctx context.Context,
	tx pgx.Tx,
	eventID string,
	tenantID types.TenantID,
	aggregateType string,
	aggregateID string,
	eventType string,
	partitionKey string,
	payload map[string]any,
) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO presence_outbox (
    event_id, tenant_id, aggregate_type, aggregate_id, event_type, event_version,
    partition_key, payload_json, status, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, 1, $6, $7::jsonb, 'PENDING', now(), now())
`, eventID, string(tenantID), aggregateType, aggregateID, eventType, partitionKey, string(encoded))
	if err != nil {
		return fmt.Errorf("insert presence outbox: %w", err)
	}
	return nil
}
