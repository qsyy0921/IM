package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/notification-service/internal/types"
)

func TestDeliveryStoreMarksSucceededIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openNotificationTestPool(t)
	resetNotificationTables(t, ctx, pool)
	repository := NewRepository(pool)
	store := NewDeliveryStore(pool)

	command := createNotificationCommand()
	result, err := repository.CreateNotificationRequest(ctx, command, "notif-delivery-success", "hash-user-example", command.CommandHash("hash-user-example"))
	if err != nil {
		t.Fatalf("create notification request: %v", err)
	}
	requests, err := store.ClaimReadyDeliveryRequests(ctx, 10, "local-noop")
	if err != nil {
		t.Fatalf("claim ready deliveries: %v", err)
	}
	if len(requests) != 1 || requests[0].RequestID != result.RequestID || requests[0].AttemptNumber != 1 {
		t.Fatalf("unexpected claimed requests: %+v", requests)
	}
	if err := store.MarkDeliverySucceeded(ctx, requests[0], types.DeliveryResult{
		ProviderID:            "local-noop",
		ProviderMessageIDHash: "provider-message-hash",
	}); err != nil {
		t.Fatalf("mark delivery succeeded: %v", err)
	}

	fetched, err := repository.GetNotificationRequest(ctx, command.AuthContext.TenantID, result.RequestID)
	if err != nil {
		t.Fatalf("get notification request: %v", err)
	}
	if fetched.Status != types.StatusDelivered || fetched.DeliveredAt.IsZero() || fetched.AttemptCount != 1 {
		t.Fatalf("unexpected delivered request: %+v", fetched)
	}
	assertDeliveryAttempt(t, ctx, pool, command.AuthContext.TenantID, result.RequestID, types.AttemptStatusSucceeded)
	assertNotificationOutboxEvent(t, ctx, pool, command.AuthContext.TenantID, result.RequestID, types.NotificationEventDeliverySucceeded)
}

func TestDeliveryStoreFailureDeadLettersIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openNotificationTestPool(t)
	resetNotificationTables(t, ctx, pool)
	repository := NewRepository(pool)
	store := NewDeliveryStore(pool, WithDeliveryClock(func() time.Time {
		return time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
	}))

	command := createNotificationCommand()
	command.IdempotencyKey = "idem-delivery-dlq"
	result, err := repository.CreateNotificationRequest(ctx, command, "notif-delivery-dlq", "hash-user-example", command.CommandHash("hash-user-example"))
	if err != nil {
		t.Fatalf("create notification request: %v", err)
	}
	requests, err := store.ClaimReadyDeliveryRequests(ctx, 10, "local-noop")
	if err != nil {
		t.Fatalf("claim ready deliveries: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("unexpected claimed requests: %+v", requests)
	}
	deadLettered, err := store.MarkDeliveryFailed(ctx, requests[0], types.NewProviderUnavailableFailure(), 1, time.Millisecond)
	if err != nil {
		t.Fatalf("mark delivery failed: %v", err)
	}
	if !deadLettered {
		t.Fatalf("expected dead-lettered failure")
	}

	fetched, err := repository.GetNotificationRequest(ctx, command.AuthContext.TenantID, result.RequestID)
	if err != nil {
		t.Fatalf("get notification request: %v", err)
	}
	if fetched.Status != types.StatusDLQ || fetched.DeadLetteredAt.IsZero() || fetched.LastFailureClass != types.FailureClassProviderUnavailable {
		t.Fatalf("unexpected DLQ request: %+v", fetched)
	}
	assertDeliveryAttempt(t, ctx, pool, command.AuthContext.TenantID, result.RequestID, types.AttemptStatusFailed)
	assertNotificationOutboxEvent(t, ctx, pool, command.AuthContext.TenantID, result.RequestID, types.NotificationEventDeliveryDeadLettered)
}

func assertDeliveryAttempt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID types.TenantID, requestID string, status string) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM notification_delivery_attempts
WHERE tenant_id = $1
  AND request_id = $2
  AND status = $3
`, tenantID, requestID, status).Scan(&count); err != nil {
		t.Fatalf("count delivery attempt: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one delivery attempt with status %s, got %d", status, count)
	}
}

func assertNotificationOutboxEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID types.TenantID, requestID string, eventType string) {
	t.Helper()
	var payload string
	if err := pool.QueryRow(ctx, `
SELECT payload_json::text
FROM notification_outbox
WHERE tenant_id = $1
  AND request_id = $2
  AND event_type = $3
`, tenantID, requestID, eventType).Scan(&payload); err != nil {
		t.Fatalf("read notification outbox event %s: %v", eventType, err)
	}
	if !stringsSafeNotificationPayload(payload) {
		t.Fatalf("notification outbox leaked unsafe payload: %s", payload)
	}
}

func stringsSafeNotificationPayload(payload string) bool {
	for _, marker := range []string{"destination_ref", "destination_hash", "secret_payload", "provider_body", "provider_response", "smtp_transcript", "reset_token", "totp", "recovery_code"} {
		if strings.Contains(strings.ToLower(payload), marker) {
			return false
		}
	}
	return true
}
