package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/notification-service/internal/types"
)

func TestOutboxStoreProcessReadyBatchPublishesAcceptedIntegration(t *testing.T) {
	pool := openNotificationTestPool(t)
	ctx := context.Background()
	resetNotificationTables(t, ctx, pool)
	repository := NewRepository(pool)

	command := createNotificationCommand()
	result, err := repository.CreateNotificationRequest(ctx, command, "notif-outbox-publish", "hash-user-example", command.CommandHash("hash-user-example"))
	if err != nil {
		t.Fatalf("create notification request: %v", err)
	}

	store := NewOutboxStore(pool)
	var published []string
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for _, message := range messages {
			published = append(published, message.EventID)
			if message.EventType != types.NotificationEventRequestAccepted || message.Producer != "notification-service" {
				t.Fatalf("unexpected outbox message: %+v", message)
			}
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process ready batch: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || len(published) != 1 || published[0] != result.RequestID+":accepted" {
		t.Fatalf("unexpected batch stats=%+v published=%v", stats, published)
	}
	assertNotificationOutboxStatus(t, ctx, pool, result.TenantID, result.RequestID+":accepted", types.OutboxStatusPublished)
}

func TestOutboxStoreDLQBlocksLaterRequestEventsIntegration(t *testing.T) {
	pool := openNotificationTestPool(t)
	ctx := context.Background()
	resetNotificationTables(t, ctx, pool)
	repository := NewRepository(pool)

	command := createNotificationCommand()
	result, err := repository.CreateNotificationRequest(ctx, command, "notif-outbox-dlq", "hash-user-example", command.CommandHash("hash-user-example"))
	if err != nil {
		t.Fatalf("create notification request: %v", err)
	}
	laterEventID := insertNotificationOutboxEvent(t, ctx, pool, result, "notif-outbox-dlq:future", 2)

	store := NewOutboxStore(pool)
	stats, err := store.ProcessReadyBatch(ctx, 10, 1, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index := range errs {
			errs[index] = errors.New("kafka broker raw failure with internal detail")
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process failing batch: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 0 || stats.DeadLettered != 1 {
		t.Fatalf("unexpected failing batch stats: %+v", stats)
	}
	assertNotificationOutboxStatus(t, ctx, pool, result.TenantID, result.RequestID+":accepted", types.OutboxStatusDLQ)
	assertNotificationOutboxStatus(t, ctx, pool, result.TenantID, laterEventID, types.OutboxStatusPending)
	assertNotificationOutboxLastError(t, ctx, pool, result.TenantID, result.RequestID+":accepted", "notification outbox publish broker unavailable")

	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		t.Fatalf("later event must stay blocked while prior event is DLQ: %+v", messages)
		return nil
	})
	if err != nil {
		t.Fatalf("process blocked batch: %v", err)
	}
	if stats.Fetched != 0 || stats.Published != 0 {
		t.Fatalf("expected no ready rows while earlier event is DLQ, got %+v", stats)
	}
}

func TestOutboxStoreRetryKeepsStablePublicErrorIntegration(t *testing.T) {
	pool := openNotificationTestPool(t)
	ctx := context.Background()
	resetNotificationTables(t, ctx, pool)
	repository := NewRepository(pool)

	command := createNotificationCommand()
	result, err := repository.CreateNotificationRequest(ctx, command, "notif-outbox-retry", "hash-user-example", command.CommandHash("hash-user-example"))
	if err != nil {
		t.Fatalf("create notification request: %v", err)
	}

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	}))
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index := range errs {
			errs[index] = errors.New("duplicate key value violates unique constraint notification_secret")
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process retry batch: %v", err)
	}
	eventID := result.RequestID + ":accepted"
	if stats.Fetched != 1 || stats.Retried != 1 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected retry stats: %+v", stats)
	}
	assertNotificationOutboxStatus(t, ctx, pool, result.TenantID, eventID, types.OutboxStatusPending)
	assertNotificationOutboxRetry(t, ctx, pool, result.TenantID, eventID, 1)
	assertNotificationOutboxLastError(t, ctx, pool, result.TenantID, eventID, "notification outbox publish failed")
}

func insertNotificationOutboxEvent(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	request types.NotificationRequest,
	eventID string,
	eventVersion int,
) string {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO notification_outbox (
    event_id,
    tenant_id,
    request_id,
    event_type,
    event_version,
    partition_key,
    payload_json,
    status,
    created_at,
    updated_at
) VALUES ($1, $2, $3, 'notification.request.accepted.v1', $4, $5, $6::jsonb, 'PENDING', now() + interval '1 second', now())
`, eventID, request.TenantID, request.RequestID, eventVersion, string(request.TenantID)+":"+request.RequestID, notificationOutboxPayload(request))
	if err != nil {
		t.Fatalf("insert notification outbox event: %v", err)
	}
	return eventID
}

func notificationOutboxPayload(request types.NotificationRequest) string {
	return `{"tenant_id":"` + string(request.TenantID) + `","request_id":"` + request.RequestID + `","requester_service":"` + request.RequesterService + `","channel":"` + request.Channel + `","recipient_ref":"` + request.RecipientRef + `","destination_masked":"` + request.DestinationMasked + `","template_key":"` + request.TemplateKey + `","template_version":"` + request.TemplateVersion + `","locale":"` + request.Locale + `","priority":"` + request.Priority + `","status":"` + request.Status + `"}`
}

func assertNotificationOutboxStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID types.TenantID, eventID string, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT status FROM notification_outbox WHERE tenant_id = $1 AND event_id = $2`, tenantID, eventID).Scan(&got); err != nil {
		t.Fatalf("query notification outbox status: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected notification outbox status for %s: got %s want %s", eventID, got, want)
	}
}

func assertNotificationOutboxRetry(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID types.TenantID, eventID string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT retry_count FROM notification_outbox WHERE tenant_id = $1 AND event_id = $2`, tenantID, eventID).Scan(&got); err != nil {
		t.Fatalf("query notification outbox retry count: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected notification outbox retry count for %s: got %d want %d", eventID, got, want)
	}
}

func assertNotificationOutboxLastError(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID types.TenantID, eventID string, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT last_error FROM notification_outbox WHERE tenant_id = $1 AND event_id = $2`, tenantID, eventID).Scan(&got); err != nil {
		t.Fatalf("query notification outbox last_error: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected notification outbox last_error for %s: got %q want %q", eventID, got, want)
	}
}
