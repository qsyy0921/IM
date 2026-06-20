package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/presence-service/internal/domain"
	"github.com/qsyy0921/IM/services/presence-service/internal/types"
)

func TestRepositoryUpdatePresenceTypingIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openPresenceTestPool(t)
	resetPresenceTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := preparePresenceUpdate(t, "idem-1", types.PresenceStateOnline)
	state, err := repository.UpdatePresence(ctx, prepared, "evt_presence_1")
	if err != nil {
		t.Fatalf("update presence: %v", err)
	}
	if state.ActualState != types.PresenceStateOnline || state.VisibleState != types.PresenceStateOnline || state.DeviceCount != 1 {
		t.Fatalf("unexpected state: %+v", state)
	}

	replay, err := repository.UpdatePresence(ctx, prepared, "evt_presence_replay")
	if err != nil {
		t.Fatalf("replay presence: %v", err)
	}
	if replay.UserID != state.UserID || replay.ActualState != state.ActualState {
		t.Fatalf("unexpected replay: %+v", replay)
	}

	conflict := preparePresenceUpdate(t, "idem-1", types.PresenceStateAway)
	if _, err := repository.UpdatePresence(ctx, conflict, "evt_presence_conflict"); !errors.Is(err, types.ErrAlreadyExists) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	states, err := repository.GetPresenceStates(ctx, types.GetPresenceCommand{
		AuthContext:    validAuth("user-1"),
		TargetUserIDs:  []string{"user-1"},
		IncludeDevices: true,
	})
	if err != nil {
		t.Fatalf("get presence states: %v", err)
	}
	if len(states) != 1 || len(states[0].DeviceStates) != 1 || states[0].DeviceStates[0].SessionID != "session-1" {
		t.Fatalf("unexpected loaded states: %+v", states)
	}

	typing, err := repository.UpdateTyping(ctx, prepareTypingUpdate(t), "evt_typing_1")
	if err != nil {
		t.Fatalf("update typing: %v", err)
	}
	if typing.TypingState != types.TypingStateStarted || typing.ExpiresAt.IsZero() {
		t.Fatalf("unexpected typing: %+v", typing)
	}
	assertPresenceOutboxLowSensitive(t, ctx, pool)
}

func TestRepositoryInvisibleStatePersistsActualButVisibleOfflineIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openPresenceTestPool(t)
	resetPresenceTables(t, ctx, pool)
	repository := NewRepository(pool)

	state, err := repository.UpdatePresence(ctx, preparePresenceUpdate(t, "idem-invisible", types.PresenceStateInvisible), "evt_presence_invisible")
	if err != nil {
		t.Fatalf("update invisible presence: %v", err)
	}
	if state.ActualState != types.PresenceStateInvisible || state.VisibleState != types.PresenceStateOffline {
		t.Fatalf("expected invisible actual/offline visible, got %+v", state)
	}
}

func preparePresenceUpdate(t *testing.T, idempotencyKey string, state string) domain.PreparedPresenceUpdate {
	t.Helper()
	prepared, err := domain.PreparePresenceUpdate(types.UpdatePresenceCommand{
		AuthContext:    validAuth("user-1"),
		UserID:         "user-1",
		DeviceID:       "device-1",
		SessionID:      "session-1",
		PresenceState:  state,
		ManualStatus:   "available",
		TTL:            time.Minute,
		Source:         types.SourceClient,
		IdempotencyKey: idempotencyKey,
		CorrelationID:  "corr-1",
		TraceID:        "trace-1",
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare presence update: %v", err)
	}
	return prepared
}

func prepareTypingUpdate(t *testing.T) domain.PreparedTypingUpdate {
	t.Helper()
	prepared, err := domain.PrepareTypingUpdate(types.UpdateTypingCommand{
		AuthContext:    validAuth("user-1"),
		ConversationID: "conversation-1",
		UserID:         "user-1",
		DeviceID:       "device-1",
		TypingState:    types.TypingStateStarted,
		TTL:            15 * time.Second,
		CorrelationID:  "corr-typing",
		TraceID:        "trace-typing",
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare typing update: %v", err)
	}
	return prepared
}

func validAuth(userID string) types.AuthContext {
	return types.AuthContext{
		TenantID: "tenant-presence-test",
		UserID:   userID,
		TraceID:  "trace-test",
	}
}

func assertPresenceOutboxLowSensitive(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `
SELECT aggregate_id, partition_key, payload_json::text
FROM presence_outbox
WHERE tenant_id = 'tenant-presence-test'
ORDER BY created_at ASC
`)
	if err != nil {
		t.Fatalf("read presence outbox: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var aggregateID string
		var partitionKey string
		var payload string
		if err := rows.Scan(&aggregateID, &partitionKey, &payload); err != nil {
			t.Fatalf("scan presence outbox: %v", err)
		}
		count++
		for _, forbidden := range []string{"user-1", "device-1", "session-1", "conversation-1", "available", "token", "password"} {
			if strings.Contains(payload, forbidden) || strings.Contains(aggregateID, forbidden) || strings.Contains(partitionKey, forbidden) {
				t.Fatalf("presence outbox leaked forbidden value %q: aggregate=%s partition=%s payload=%s", forbidden, aggregateID, partitionKey, payload)
			}
		}
		if !strings.Contains(payload, "sha256:") {
			t.Fatalf("presence outbox payload missing hashed refs: %s", payload)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("presence outbox rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected two outbox rows for first update and typing, got %d", count)
	}
}

func openPresenceTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyPresenceMigration(t, context.Background(), pool)
	return pool
}

func applyPresenceMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "presence", "000001_presence_core.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read presence migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(content)); err != nil {
		t.Fatalf("apply presence migration: %v", err)
	}
}

func resetPresenceTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    presence_outbox,
    presence_typing_indicators,
    presence_subscriptions,
    presence_sessions,
    presence_user_states
RESTART IDENTITY CASCADE
`)
	if err != nil {
		t.Fatalf("reset presence tables: %v", err)
	}
}
