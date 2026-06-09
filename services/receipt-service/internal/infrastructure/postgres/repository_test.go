package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

func TestRepositoryMessageChangeEventsDoNotIncreaseUnreadIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	if _, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1")); err != nil {
		t.Fatalf("project persisted inbox item: %v", err)
	}
	if _, err := repository.ProjectDeliveryEvent(ctx, ackRecordedCommand(1, "delivery-ack-1")); err != nil {
		t.Fatalf("project ack: %v", err)
	}
	if _, err := repository.MarkRead(ctx, markReadCommand(1)); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	for _, eventType := range []string{
		types.SourceEventMessageEdited,
		types.SourceEventMessageRevoked,
		types.SourceEventMessageDeleted,
	} {
		seq := int64(2)
		if eventType == types.SourceEventMessageRevoked {
			seq = 3
		}
		if eventType == types.SourceEventMessageDeleted {
			seq = 4
		}
		command := inboxCreatedCommand(seq, "delivery-"+eventType)
		command.SourceEventID = fmt.Sprintf("timeline-change-%d", seq)
		command.SourceEventType = eventType
		command.MessageID = "message-1"
		if _, err := repository.ProjectDeliveryEvent(ctx, command); err != nil {
			t.Fatalf("project %s: %v", eventType, err)
		}
	}

	summary, err := repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list conversations after message changes: %v", err)
	}
	assertConversationSummaryWithMessage(t, summary, 4, "message-1", types.SourceEventMessageDeleted, 0, 1)
	assertReceiptOutboxCount(t, ctx, pool, "receipt.message.received.v1", 1)
	assertReceiptOutboxCount(t, ctx, pool, "receipt.message.read.v1", 1)
	assertReceiptStateCount(t, ctx, pool, 1)
}

func TestRepositoryListConversationsPaginatesByStableCursorIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	sortTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	insertConversationSummary(t, ctx, pool, "conv-a", 11, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-b", 12, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-c", 13, sortTime.Add(-time.Minute))

	first, err := repository.ListConversations(ctx, listConversationsCommand(1, ""))
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	assertConversationIDs(t, first, "conv-a")
	if first.NextPageCursor == "" {
		t.Fatal("expected next cursor for first page")
	}

	second, err := repository.ListConversations(ctx, listConversationsCommand(1, first.NextPageCursor))
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}
	assertConversationIDs(t, second, "conv-b")
	if second.NextPageCursor == "" {
		t.Fatal("expected next cursor for second page")
	}

	third, err := repository.ListConversations(ctx, listConversationsCommand(1, second.NextPageCursor))
	if err != nil {
		t.Fatalf("list third page: %v", err)
	}
	assertConversationIDs(t, third, "conv-c")
	if third.NextPageCursor != "" {
		t.Fatalf("expected empty next cursor on last page, got %q", third.NextPageCursor)
	}
}

func TestRepositoryListConversationsRejectsInvalidCursorIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ListConversations(ctx, listConversationsCommand(10, "not-a-valid-cursor"))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}

	mismatchedCursor := encodeTestListCursor(t, map[string]any{
		"v":               1,
		"sort":            "other_sort",
		"sort_updated_at": time.Now().UTC(),
		"conversation_id": "conv-a",
	})
	_, err = repository.ListConversations(ctx, listConversationsCommand(10, mismatchedCursor))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument for mismatched cursor sort, got %v", err)
	}
}

func TestRepositoryArchiveConversationFiltersListIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	if _, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1")); err != nil {
		t.Fatalf("project inbox item: %v", err)
	}
	archiveResult, err := repository.ArchiveConversation(ctx, archiveConversationCommand(true))
	if err != nil {
		t.Fatalf("archive conversation: %v", err)
	}
	if !archiveResult.Conversation.Archived {
		t.Fatalf("expected archived conversation in response: %+v", archiveResult)
	}

	defaultList, err := repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list default conversations: %v", err)
	}
	assertConversationIDs(t, defaultList)

	archivedList, err := repository.ListConversations(ctx, listConversationsCommandIncludingArchived(10, ""))
	if err != nil {
		t.Fatalf("list including archived conversations: %v", err)
	}
	assertConversationSummaryWithArchive(t, archivedList, 1, true)

	if _, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(2, "delivery-inbox-2")); err != nil {
		t.Fatalf("project inbox while archived: %v", err)
	}
	defaultList, err = repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list default conversations after new event: %v", err)
	}
	assertConversationIDs(t, defaultList)
	archivedList, err = repository.ListConversations(ctx, listConversationsCommandIncludingArchived(10, ""))
	if err != nil {
		t.Fatalf("list including archived conversations after new event: %v", err)
	}
	assertConversationSummaryWithArchive(t, archivedList, 2, true)

	unarchiveResult, err := repository.ArchiveConversation(ctx, archiveConversationCommand(false))
	if err != nil {
		t.Fatalf("unarchive conversation: %v", err)
	}
	if unarchiveResult.Conversation.Archived {
		t.Fatalf("expected unarchived conversation in response: %+v", unarchiveResult)
	}
	defaultList, err = repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list default conversations after unarchive: %v", err)
	}
	assertConversationSummaryWithArchive(t, defaultList, 2, false)
}

func TestRepositoryArchiveConversationRejectsUnknownSummaryIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ArchiveConversation(ctx, archiveConversationCommand(true))
	if !errors.Is(err, types.ErrConversationNotFound) {
		t.Fatalf("expected conversation not found, got %v", err)
	}
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
		SourceEventType: types.SourceEventMessagePersisted,
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

func insertConversationSummary(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	conversationID string,
	lastVisibleSeq int64,
	sortUpdatedAt time.Time,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO user_conversation_summaries (
    tenant_id,
    user_id,
    conversation_id,
    last_visible_seq,
    last_message_id,
    last_sender_id,
    last_source_event_type,
    last_read_seq,
    unread_count,
    sort_updated_at,
    updated_at
) VALUES (
    'tenant-receipt',
    'receiver-1',
    $1,
    $2,
    $3,
    'sender-1',
    'message.persisted.v1',
    0,
    1,
    $4,
    $4
)
`, conversationID, lastVisibleSeq, fmt.Sprintf("message-%d", lastVisibleSeq), sortUpdatedAt)
	if err != nil {
		t.Fatalf("insert conversation summary %s: %v", conversationID, err)
	}
}

func encodeTestListCursor(t *testing.T, value map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode test cursor: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
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

func listConversationsCommandIncludingArchived(limit int, cursor string) types.ListConversationsCommand {
	command := listConversationsCommand(limit, cursor)
	command.IncludeArchived = true
	return command
}

func archiveConversationCommand(archived bool) types.ArchiveConversationCommand {
	return types.ArchiveConversationCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-receipt",
			UserID:   "receiver-1",
			DeviceID: "device-1",
		},
		ConversationID: "conv-receipt",
		Archived:       archived,
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

func assertConversationIDs(t *testing.T, result types.ListConversationsResult, want ...types.ConversationID) {
	t.Helper()
	if len(result.Items) != len(want) {
		t.Fatalf("expected %d conversation summaries, got %d: %+v", len(want), len(result.Items), result.Items)
	}
	for index, conversationID := range want {
		if result.Items[index].ConversationID != conversationID {
			t.Fatalf("expected item %d conversation_id=%s, got %s", index, conversationID, result.Items[index].ConversationID)
		}
	}
}

func assertConversationSummary(
	t *testing.T,
	result types.ListConversationsResult,
	wantLastVisibleSeq int64,
	wantUnread int64,
	wantLastReadSeq int64,
) {
	t.Helper()
	assertConversationSummaryWithMessage(
		t,
		result,
		wantLastVisibleSeq,
		fmt.Sprintf("message-%d", wantLastVisibleSeq),
		types.SourceEventMessagePersisted,
		wantUnread,
		wantLastReadSeq,
	)
}

func assertConversationSummaryWithMessage(
	t *testing.T,
	result types.ListConversationsResult,
	wantLastVisibleSeq int64,
	wantLastMessageID string,
	wantLastSourceEventType string,
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
	if item.LastMessageID != wantLastMessageID {
		t.Fatalf("unexpected last_message_id: %+v", item)
	}
	if item.LastSourceEventType != wantLastSourceEventType {
		t.Fatalf("unexpected last_source_event_type: %+v", item)
	}
}

func assertConversationSummaryWithArchive(
	t *testing.T,
	result types.ListConversationsResult,
	wantLastVisibleSeq int64,
	wantArchived bool,
) {
	t.Helper()
	assertConversationSummary(t, result, wantLastVisibleSeq, wantLastVisibleSeq, 0)
	if result.Items[0].Archived != wantArchived {
		t.Fatalf("expected archived=%v, got %+v", wantArchived, result.Items[0])
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

func assertReceiptStateCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM message_receipt_states
WHERE tenant_id = 'tenant-receipt'
`).Scan(&got)
	if err != nil {
		t.Fatalf("count receipt states: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d receipt states, got %d", want, got)
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
		"000003_receipt_source_event_type.sql",
		"000004_conversation_summary_source_event_type.sql",
		"000005_conversation_archive.sql",
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
