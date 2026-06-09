package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func TestRepositoryProjectDeliveryEventAndMarkReadIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1"))
	if err != nil {
		t.Fatalf("project inbox item: %v", err)
	}
	_, err = repository.ProjectDeliveryEvent(ctx, ackRecordedCommand(1, "delivery-ack-1"))
	if err != nil {
		t.Fatalf("project ack: %v", err)
	}
	state, err := repository.GetReceiptState(ctx, getStateCommandBySeq(1))
	if err != nil {
		t.Fatalf("get receipt state after ack: %v", err)
	}
	if state.ReceivedUserCount != 1 || state.ReadUserCount != 0 {
		t.Fatalf("unexpected counts after ack: received=%d read=%d", state.ReceivedUserCount, state.ReadUserCount)
	}

	result, err := repository.MarkRead(ctx, markReadCommand(1))
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if result.LastReadSeq != 1 {
		t.Fatalf("expected last_read_seq=1, got %d", result.LastReadSeq)
	}
	state, err = repository.GetReceiptState(ctx, getStateCommandByMessage("message-1"))
	if err != nil {
		t.Fatalf("get receipt state after read: %v", err)
	}
	if state.ReceivedUserCount != 1 || state.ReadUserCount != 1 {
		t.Fatalf("unexpected counts after read: received=%d read=%d", state.ReceivedUserCount, state.ReadUserCount)
	}
	assertReceiptOutboxCount(t, ctx, pool, "receipt.message.received.v1", 1)
	assertReceiptOutboxCount(t, ctx, pool, "receipt.message.read.v1", 1)

	result, err = repository.MarkRead(ctx, markReadCommand(1))
	if err != nil {
		t.Fatalf("repeat mark read should be idempotent: %v", err)
	}
	if result.LastReadSeq != 1 {
		t.Fatalf("expected idempotent read seq 1, got %d", result.LastReadSeq)
	}
	assertReceiptOutboxCount(t, ctx, pool, "receipt.message.read.v1", 1)
}

func TestRepositoryMarkReadRejectsOutOfRangeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1"))
	if err != nil {
		t.Fatalf("project inbox item: %v", err)
	}
	_, err = repository.MarkRead(ctx, markReadCommand(2))
	if !errors.Is(err, types.ErrReadOutOfVisibleRange) {
		t.Fatalf("expected visible range error, got %v", err)
	}
	_, err = repository.MarkRead(ctx, markReadCommand(1))
	if !errors.Is(err, types.ErrReadOutOfReceivedRange) {
		t.Fatalf("expected received range error, got %v", err)
	}
}

func TestRepositoryProjectsAckBeforeInboxIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectDeliveryEvent(ctx, ackRecordedCommand(2, "delivery-ack-1"))
	if err != nil {
		t.Fatalf("project ack first: %v", err)
	}
	_, err = repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1"))
	if err != nil {
		t.Fatalf("project inbox item: %v", err)
	}
	state, err := repository.GetReceiptState(ctx, getStateCommandBySeq(1))
	if err != nil {
		t.Fatalf("get receipt state: %v", err)
	}
	if state.ReceivedUserCount != 1 {
		t.Fatalf("expected inbox projected as already received, got %d", state.ReceivedUserCount)
	}
	assertReceiptOutboxCount(t, ctx, pool, "receipt.message.received.v1", 1)
}

func inboxCreatedCommand(seq int64, eventID string) types.ProjectDeliveryEventCommand {
	return types.ProjectDeliveryEventCommand{
		TenantID:        "tenant-receipt",
		EventID:         eventID,
		EventType:       types.DeliveryEventInboxItemCreated,
		UserID:          "receiver-1",
		ConversationID:  "conv-receipt",
		ConversationSeq: seq,
		SourceEventID:   "timeline-event-1",
		MessageID:       "message-1",
		SenderID:        "sender-1",
		ConsumerGroup:   "receipt-test",
		Topic:           "im.delivery.events",
		PartitionID:     0,
		OffsetValue:     seq,
	}
}

func ackRecordedCommand(receivedSeq int64, eventID string) types.ProjectDeliveryEventCommand {
	return types.ProjectDeliveryEventCommand{
		TenantID:        "tenant-receipt",
		EventID:         eventID,
		EventType:       types.DeliveryEventAckRecorded,
		UserID:          "receiver-1",
		DeviceID:        "device-1",
		ConversationID:  "conv-receipt",
		LastReceivedSeq: receivedSeq,
		ConsumerGroup:   "receipt-test",
		Topic:           "im.delivery.events",
		PartitionID:     0,
		OffsetValue:     receivedSeq + 10,
	}
}

func markReadCommand(seq int64) types.MarkReadCommand {
	return types.MarkReadCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-receipt",
			UserID:   "receiver-1",
			DeviceID: "device-1",
		},
		ConversationID: "conv-receipt",
		ReadSeq:        seq,
	}
}

func getStateCommandBySeq(seq int64) types.GetReceiptStateCommand {
	return types.GetReceiptStateCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-receipt",
			UserID:   "sender-1",
			DeviceID: "device-sender",
		},
		ConversationID:  "conv-receipt",
		ConversationSeq: seq,
	}
}

func getStateCommandByMessage(messageID string) types.GetReceiptStateCommand {
	command := getStateCommandBySeq(0)
	command.MessageID = messageID
	return command
}

func assertReceiptOutboxCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventType string, want int) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM receipt_outbox
WHERE tenant_id = 'tenant-receipt'
  AND event_type = $1
`, eventType).Scan(&got)
	if err != nil {
		t.Fatalf("count receipt outbox: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d outbox rows for %s, got %d", want, eventType, got)
	}
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyReceiptMigration(t, ctx, pool)
	return pool
}

func applyReceiptMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	root := findRepoRoot(t)
	migrationPath := filepath.Join(root, "migrations", "postgres", "receipt", "000001_receipt_core.sql")
	sqlBytes, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
}

func resetReceiptTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    receipt_outbox,
    receipt_kafka_checkpoints,
    message_receipt_states,
    user_read_cursors,
    user_received_cursors,
    device_received_cursors,
    receipt_inbox_projection
RESTART IDENTITY
`)
	if err != nil {
		t.Fatalf("reset receipt tables: %v", err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("repo root not found")
		}
		wd = parent
	}
}
