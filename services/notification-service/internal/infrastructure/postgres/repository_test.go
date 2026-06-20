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
	"github.com/qsyy0921/IM/services/notification-service/internal/types"
)

func TestRepositoryCreateNotificationRequestIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openNotificationTestPool(t)
	resetNotificationTables(t, ctx, pool)
	repository := NewRepository(pool)

	command := createNotificationCommand()
	result, err := repository.CreateNotificationRequest(ctx, command, "notif-1", "hash-user-example", command.CommandHash("hash-user-example"))
	if err != nil {
		t.Fatalf("create notification request: %v", err)
	}
	if result.Status != types.StatusAccepted || result.DestinationHash != "hash-user-example" {
		t.Fatalf("unexpected request: %+v", result)
	}
	if result.DestinationMasked != "u***@example.com" {
		t.Fatalf("unexpected masked destination: %+v", result)
	}

	replay, err := repository.CreateNotificationRequest(ctx, command, "notif-should-not-win", "hash-user-example", command.CommandHash("hash-user-example"))
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if replay.RequestID != result.RequestID {
		t.Fatalf("replay returned different request: %+v", replay)
	}

	conflict := command
	conflict.TemplateVersion = "v2"
	if _, err := repository.CreateNotificationRequest(ctx, conflict, "notif-conflict", "hash-user-example", conflict.CommandHash("hash-user-example")); !errors.Is(err, types.ErrAlreadyExists) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	assertNotificationOutboxDoesNotLeak(t, ctx, pool, "notif-1", command.DestinationRef, string(command.SecretPayloadCiphertext))

	fetched, err := repository.GetNotificationRequest(ctx, command.AuthContext.TenantID, "notif-1")
	if err != nil {
		t.Fatalf("get notification request: %v", err)
	}
	if fetched.RequestID != result.RequestID || fetched.DestinationHash != result.DestinationHash {
		t.Fatalf("unexpected fetched request: %+v", fetched)
	}

	canceled, err := repository.CancelNotificationRequest(ctx, types.CancelNotificationRequestCommand{
		AuthContext:     command.AuthContext,
		RequestID:       "notif-1",
		CancelRequestID: "cancel-1",
	})
	if err != nil {
		t.Fatalf("cancel notification request: %v", err)
	}
	if canceled.Status != types.StatusCanceled || canceled.CanceledAt.IsZero() {
		t.Fatalf("unexpected canceled request: %+v", canceled)
	}
}

func createNotificationCommand() types.CreateNotificationRequestCommand {
	return types.CreateNotificationRequestCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-notification-test",
			UserID:   "user-1",
			DeviceID: "device-1",
			TraceID:  "trace-1",
		},
		RequesterService:        "identity-service",
		RequesterUserID:         "user-1",
		Channel:                 types.ChannelEmail,
		RecipientRef:            "user:user-1",
		DestinationRef:          "user@example.com",
		DestinationMasked:       "u***@example.com",
		TemplateKey:             "verify-email",
		TemplateVersion:         "v1",
		Priority:                types.PriorityNormal,
		IdempotencyKey:          "idem-1",
		TemplateVariablesJSON:   `{"display_name":"User"}`,
		SecretPayloadCiphertext: []byte("encrypted-secret-token"),
		SecretPayloadKeyVersion: "local-v1",
		SecretPayloadExpiresAt:  time.Now().Add(10 * time.Minute),
		CorrelationID:           "corr-1",
		CausationID:             "cause-1",
	}
}

func assertNotificationOutboxDoesNotLeak(t *testing.T, ctx context.Context, pool *pgxpool.Pool, requestID string, rawDestination string, rawSecret string) {
	t.Helper()
	var payload string
	if err := pool.QueryRow(ctx, `
SELECT payload_json::text
FROM notification_outbox
WHERE tenant_id = 'tenant-notification-test'
  AND request_id = $1
  AND event_type = 'notification.request.accepted.v1'
`, requestID).Scan(&payload); err != nil {
		t.Fatalf("read notification outbox payload: %v", err)
	}
	if strings.Contains(payload, rawDestination) {
		t.Fatalf("outbox payload leaked raw destination: %s", payload)
	}
	if strings.Contains(payload, rawSecret) {
		t.Fatalf("outbox payload leaked raw secret: %s", payload)
	}
}

func openNotificationTestPool(t *testing.T) *pgxpool.Pool {
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
	applyNotificationMigrations(t, context.Background(), pool)
	return pool
}

func applyNotificationMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "notification")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migration dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", entry.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			t.Fatalf("apply migration %s: %v", entry.Name(), err)
		}
	}
}

func resetNotificationTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
TRUNCATE
	notification_outbox,
	notification_delivery_attempts,
	notification_suppressions,
	notification_provider_routes,
	notification_templates,
	notification_requests
`); err != nil {
		t.Fatalf("reset notification tables: %v", err)
	}
}
