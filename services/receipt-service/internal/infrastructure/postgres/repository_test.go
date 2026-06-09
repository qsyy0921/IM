package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	summary, err := repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list conversations before read: %v", err)
	}
	assertConversationSummary(t, summary, 1, 1, 0)

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
	summary, err = repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list conversations after read: %v", err)
	}
	assertConversationSummary(t, summary, 1, 0, 1)
	assertReceiptOutboxCount(t, ctx, pool, "receipt.message.received.v1", 1)
	assertReceiptOutboxCount(t, ctx, pool, "receipt.message.read.v1", 1)
	assertReceiptOutboxPayload(t, ctx, pool, "receipt.message.received.v1", "message-1", "timeline-event-1", "device-1", 1)
	assertReceiptOutboxPayload(t, ctx, pool, "receipt.message.read.v1", "message-1", "timeline-event-1", "device-1", 1)

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
	assertReceiptOutboxPayload(t, ctx, pool, "receipt.message.received.v1", "message-1", "timeline-event-1", "device-1", 1)
}

func TestRepositoryListConversationsConcurrentInboxAndMarkReadIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	if _, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1")); err != nil {
		t.Fatalf("project first inbox item: %v", err)
	}
	if _, err := repository.ProjectDeliveryEvent(ctx, ackRecordedCommand(1, "delivery-ack-1")); err != nil {
		t.Fatalf("project first ack: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(2, "delivery-inbox-2"))
		errs <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		_, err := repository.MarkRead(ctx, markReadCommand(1))
		errs <- err
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent operation failed: %v", err)
		}
	}

	summary, err := repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list conversations after concurrent update: %v", err)
	}
	assertConversationSummary(t, summary, 2, 1, 1)
}

func inboxCreatedCommand(seq int64, eventID string) types.ProjectDeliveryEventCommand {
	return types.ProjectDeliveryEventCommand{
		TenantID:        "tenant-receipt",
		EventID:         eventID,
		EventType:       types.DeliveryEventInboxItemCreated,
		UserID:          "receiver-1",
		ConversationID:  "conv-receipt",
		ConversationSeq: seq,
		SourceEventID:   fmt.Sprintf("timeline-event-%d", seq),
		MessageID:       fmt.Sprintf("message-%d", seq),
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

func listConversationsCommand(limit int, cursor string) types.ListConversationsCommand {
	return types.ListConversationsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-receipt",
			UserID:   "receiver-1",
			DeviceID: "device-1",
		},
		Limit:      limit,
		PageCursor: cursor,
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

func assertConversationSummary(
	t *testing.T,
	result types.ListConversationsResult,
	wantLastVisibleSeq int64,
	wantUnread int64,
	wantLastReadSeq int64,
) {
	t.Helper()
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 conversation summary, got %d: %+v", len(result.Items), result.Items)
	}
	item := result.Items[0]
	if item.ConversationID != "conv-receipt" ||
		item.LastVisibleSeq != wantLastVisibleSeq ||
		item.UnreadCount != wantUnread ||
		item.LastReadSeq != wantLastReadSeq {
		t.Fatalf("unexpected conversation summary: %+v", item)
	}
	if item.LastMessageID != fmt.Sprintf("message-%d", wantLastVisibleSeq) {
		t.Fatalf("unexpected last_message_id: %+v", item)
	}
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

func assertReceiptOutboxPayload(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	eventType string,
	wantMessageID string,
	wantSourceEventID string,
	wantDeviceID string,
	wantCursorSeq int64,
) {
	t.Helper()
	var raw []byte
	err := pool.QueryRow(ctx, `
SELECT payload_json
FROM receipt_outbox
WHERE tenant_id = 'tenant-receipt'
  AND event_type = $1
ORDER BY id DESC
LIMIT 1
`, eventType).Scan(&raw)
	if err != nil {
		t.Fatalf("query receipt outbox payload: %v", err)
	}
	var payload struct {
		MessageID     string `json:"message_id"`
		SourceEventID string `json:"source_event_id"`
		DeviceID      string `json:"device_id"`
		CursorSeq     int64  `json:"cursor_seq"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode receipt outbox payload: %v", err)
	}
	if payload.MessageID != wantMessageID ||
		payload.SourceEventID != wantSourceEventID ||
		payload.DeviceID != wantDeviceID ||
		payload.CursorSeq != wantCursorSeq {
		t.Fatalf("unexpected payload: %+v", payload)
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
	for _, name := range []string{
		"000001_receipt_core.sql",
		"000002_conversation_summary.sql",
	} {
		migrationPath := filepath.Join(root, "migrations", "postgres", "receipt", name)
		sqlBytes, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
}

func resetReceiptTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    conversation_summary_checkpoints,
    user_conversation_summaries,
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
