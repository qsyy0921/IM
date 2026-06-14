package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestSanitizeIdentityOutboxPublishErrorUsesStablePublicMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		text string
		want string
	}{
		{
			name: "context canceled",
			err:  context.Canceled,
			text: "context canceled for user=user1@example.com token=secret-token",
			want: "identity outbox publish canceled",
		},
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			text: "deadline exceeded while publishing request-token=secret-token",
			want: "identity outbox publish timeout",
		},
		{
			name: "unsupported event",
			err:  errors.New("unsupported event_type=identity.future.v9 user=user1@example.com"),
			text: "unsupported event_type=identity.future.v9 user=user1@example.com",
			want: "identity outbox publish unsupported event",
		},
		{
			name: "invalid payload",
			err:  errors.New("malformed json payload for user=user1@example.com token=secret-token"),
			text: "malformed json payload for user=user1@example.com token=secret-token",
			want: "identity outbox publish invalid payload",
		},
		{
			name: "broker unavailable",
			err:  errors.New("kafka broker connection refused at 10.0.0.8 token=secret-token"),
			text: "kafka broker connection refused at 10.0.0.8 token=secret-token",
			want: "identity outbox publish broker unavailable",
		},
		{
			name: "unknown raw error",
			err:  errors.New("provider body user=user1@example.com token=secret-token nonce=secret-nonce"),
			text: "provider body user=user1@example.com token=secret-token nonce=secret-nonce",
			want: "identity outbox publish failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeIdentityOutboxPublishError(tt.err); got != tt.want {
				t.Fatalf("sanitize publish error = %q, want %q", got, tt.want)
			}
			if got := sanitizeIdentityOutboxStoredError(tt.text); got != tt.want {
				t.Fatalf("sanitize stored error = %q, want %q", got, tt.want)
			}
			for _, forbidden := range []string{"user1@example.com", "secret-token", "secret-nonce", "10.0.0.8"} {
				if strings.Contains(tt.want, forbidden) {
					t.Fatalf("stable identity outbox error leaked sensitive text %q in %q", forbidden, tt.want)
				}
			}
		})
	}
	if got := sanitizeIdentityOutboxStoredError("   "); got != "" {
		t.Fatalf("blank stored error = %q, want empty", got)
	}
}

func TestOutboxStoreProcessReadyBatchSanitizesPublishErrorsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)

	seedIdentityOutbox(t, ctx, pool, "identity-outbox-retry", 1, types.OutboxStatusPending, 0)
	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	}))
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		if len(messages) != 1 {
			t.Fatalf("expected one identity outbox message, got %d", len(messages))
		}
		return []error{errors.New("kafka unavailable: broker body user=user1@example.com token=secret-token")}
	})
	if err != nil {
		t.Fatalf("process retry identity outbox: %v", err)
	}
	if stats.Fetched != 1 || stats.Retried != 1 || stats.DeadLettered != 0 || stats.Published != 0 {
		t.Fatalf("unexpected retry stats: %+v", stats)
	}
	assertIdentityOutboxState(t, ctx, pool, "identity-outbox-retry", types.OutboxStatusPending, 1, "identity outbox publish broker unavailable")
	assertIdentityOutboxLastErrorDoesNotContain(t, ctx, pool, "identity-outbox-retry", "user1@example.com", "secret-token", "broker body")

	resetIdentityTables(t, ctx, pool)
	seedIdentityOutbox(t, ctx, pool, "identity-outbox-dlq", 1, types.OutboxStatusPending, 0)
	stats, err = store.ProcessReadyBatch(ctx, 10, 1, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		return []error{errors.New("malformed payload with token=secret-token")}
	})
	if err != nil {
		t.Fatalf("process dlq identity outbox: %v", err)
	}
	if stats.Fetched != 1 || stats.Retried != 0 || stats.DeadLettered != 1 || stats.Published != 0 {
		t.Fatalf("unexpected dlq stats: %+v", stats)
	}
	assertIdentityOutboxState(t, ctx, pool, "identity-outbox-dlq", types.OutboxStatusDLQ, 1, "identity outbox publish invalid payload")
	assertIdentityOutboxLastErrorDoesNotContain(t, ctx, pool, "identity-outbox-dlq", "secret-token", "malformed payload")
}

func seedIdentityOutbox(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, version int64, status string, retryCount int) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO identity_outbox (
    event_id,
    tenant_id,
    aggregate_type,
    aggregate_id,
    aggregate_version,
    event_type,
    event_version,
    mapping_version,
    partition_key,
    producer,
    correlation_id,
    causation_id,
    trace_id,
    payload_json,
    status,
    retry_count
) VALUES (
    $1,
    'tenant-identity',
    'identity_session',
    'user-1:device-1:session-1',
    $2,
    'identity.session.revoked.v1',
    'v1',
    1,
    'tenant-identity:user-1:device-1',
    'identity-service',
    'request-1',
    'request-1',
    'trace-1',
    '{"tenant_id":"tenant-identity","user_id":"user-1","device_id":"device-1","session_id":"session-1","status":"REVOKED","revoked_by":"admin-1","reason":"manual","revoked_at":"2026-06-15T12:00:00Z"}'::jsonb,
    $3,
    $4
)
`, eventID, version, status, retryCount)
	if err != nil {
		t.Fatalf("seed identity outbox: %v", err)
	}
}

func assertIdentityOutboxState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, wantStatus string, wantRetry int, wantLastError string) {
	t.Helper()
	var status string
	var retryCount int
	var lastError string
	err := pool.QueryRow(ctx, `
SELECT status, retry_count, COALESCE(last_error, '')
FROM identity_outbox
WHERE event_id = $1
`, eventID).Scan(&status, &retryCount, &lastError)
	if err != nil {
		t.Fatalf("read identity outbox state: %v", err)
	}
	if status != wantStatus || retryCount != wantRetry || lastError != wantLastError {
		t.Fatalf("unexpected identity outbox state: status=%s retry=%d last_error=%q want status=%s retry=%d last_error=%q",
			status, retryCount, lastError, wantStatus, wantRetry, wantLastError)
	}
}

func assertIdentityOutboxLastErrorDoesNotContain(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, forbidden ...string) {
	t.Helper()
	var lastError string
	if err := pool.QueryRow(ctx, `
SELECT COALESCE(last_error, '')
FROM identity_outbox
WHERE event_id = $1
`, eventID).Scan(&lastError); err != nil {
		t.Fatalf("read identity outbox last_error: %v", err)
	}
	for _, text := range forbidden {
		if strings.Contains(lastError, text) {
			t.Fatalf("identity outbox last_error leaked %q: %q", text, lastError)
		}
	}
}
