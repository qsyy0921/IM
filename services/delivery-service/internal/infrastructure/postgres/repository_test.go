package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

func TestRepositoryProjectTimelineEventIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-joined-1",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-delivery",
		ConversationSeq:   1,
		MemberUserID:      "user-1",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     1,
		PermissionVersion: 1,
		ConsumerGroup:     "delivery-test",
		Topic:             "conversation.timeline.events",
		PartitionID:       2,
		OffsetValue:       11,
	})
	if err != nil {
		t.Fatalf("project join: %v", err)
	}
	result, err := repository.ProjectTimelineEvent(ctx, messageEvent("message-1", "msg-1", 2))
	if err != nil {
		t.Fatalf("project first message: %v", err)
	}
	if result.ProjectedInboxCount != 1 {
		t.Fatalf("expected 1 projected inbox item, got %d", result.ProjectedInboxCount)
	}
	_, err = repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-left-1",
		EventType:         types.TimelineEventConversationMemberLeft,
		ConversationID:    "conv-delivery",
		ConversationSeq:   3,
		MemberUserID:      "user-1",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusLeft,
		MemberVersion:     2,
		PermissionVersion: 2,
	})
	if err != nil {
		t.Fatalf("project leave: %v", err)
	}
	result, err = repository.ProjectTimelineEvent(ctx, messageEvent("message-2", "msg-2", 4))
	if err != nil {
		t.Fatalf("project message while left: %v", err)
	}
	if result.ProjectedInboxCount != 0 {
		t.Fatalf("expected no inbox while left, got %d", result.ProjectedInboxCount)
	}
	_, err = repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-joined-2",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-delivery",
		ConversationSeq:   5,
		MemberUserID:      "user-1",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     3,
		PermissionVersion: 3,
	})
	if err != nil {
		t.Fatalf("project rejoin: %v", err)
	}
	result, err = repository.ProjectTimelineEvent(ctx, messageEvent("message-3", "msg-3", 6))
	if err != nil {
		t.Fatalf("project message after rejoin: %v", err)
	}
	if result.ProjectedInboxCount != 1 {
		t.Fatalf("expected inbox after rejoin, got %d", result.ProjectedInboxCount)
	}

	var inboxCount int
	err = pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND user_id = 'user-1'
  AND conversation_id = 'conv-delivery'
`).Scan(&inboxCount)
	if err != nil {
		t.Fatalf("count inbox: %v", err)
	}
	if inboxCount != 2 {
		t.Fatalf("expected 2 inbox rows, got %d", inboxCount)
	}
	var joinSeq int64
	var leaveSeq *int64
	var status string
	err = pool.QueryRow(ctx, `
SELECT join_seq, leave_seq, status
FROM delivery_membership_projection
WHERE tenant_id = 'tenant-delivery'
  AND conversation_id = 'conv-delivery'
  AND user_id = 'user-1'
`).Scan(&joinSeq, &leaveSeq, &status)
	if err != nil {
		t.Fatalf("read membership projection: %v", err)
	}
	if joinSeq != 5 || leaveSeq != nil || status != types.DeliveryMemberStatusActive {
		t.Fatalf("unexpected membership projection join_seq=%d leave_seq=%v status=%s", joinSeq, leaveSeq, status)
	}
	var checkpoint int64
	err = pool.QueryRow(ctx, `
SELECT offset_value
FROM delivery_kafka_checkpoints
WHERE consumer_group = 'delivery-test'
  AND topic = 'conversation.timeline.events'
  AND partition_id = 2
`).Scan(&checkpoint)
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	if checkpoint != 11 {
		t.Fatalf("expected checkpoint 11, got %d", checkpoint)
	}

	result, err = repository.ProjectTimelineEvent(ctx, messageEvent("message-3", "msg-3", 6))
	if err != nil {
		t.Fatalf("replay message: %v", err)
	}
	if result.ProjectedInboxCount != 1 {
		t.Fatalf("replay still sees one target, got %d", result.ProjectedInboxCount)
	}
	err = pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND user_id = 'user-1'
  AND event_id = 'message-3'
`).Scan(&inboxCount)
	if err != nil {
		t.Fatalf("count replay inbox: %v", err)
	}
	if inboxCount != 1 {
		t.Fatalf("expected replay to keep one inbox row, got %d", inboxCount)
	}
}

func TestRepositoryAckDeliveryIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedInbox(t, ctx, pool, 1)
	seedInbox(t, ctx, pool, 2)
	repository := NewRepository(pool)
	command := ackCommand(3)
	_, err := repository.AckDelivery(ctx, command)
	if !errors.Is(err, types.ErrAckOutOfVisibleRange) {
		t.Fatalf("expected out of visible range, got %v", err)
	}
	assertDeliveryCursor(t, ctx, pool, 0)

	result, err := repository.AckDelivery(ctx, ackCommand(2))
	if err != nil {
		t.Fatalf("ack visible seq: %v", err)
	}
	if result.LastReceivedSeq != 2 {
		t.Fatalf("expected cursor 2, got %d", result.LastReceivedSeq)
	}
	assertDeliveryCursor(t, ctx, pool, 2)
	assertDeliveryOutboxCount(t, ctx, pool, "delivery.ack.recorded.v1", 1)

	result, err = repository.AckDelivery(ctx, ackCommand(1))
	if err != nil {
		t.Fatalf("repeat lower ack should be idempotent: %v", err)
	}
	if result.LastReceivedSeq != 2 {
		t.Fatalf("expected cursor to stay 2, got %d", result.LastReceivedSeq)
	}
	assertDeliveryOutboxCount(t, ctx, pool, "delivery.ack.recorded.v1", 1)
}

func TestRepositoryAckDeliveryConcurrentFirstAckIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedInbox(t, ctx, pool, 5)
	repository := NewRepository(pool)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repository.AckDelivery(ctx, ackCommand(5))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent ack failed: %v", err)
		}
	}
	assertDeliveryCursor(t, ctx, pool, 5)
	assertDeliveryOutboxCount(t, ctx, pool, "delivery.ack.recorded.v1", 1)
}

func messageEvent(eventID string, messageID string, seq int64) types.ProjectTimelineEventCommand {
	return types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           eventID,
		EventType:         types.TimelineEventMessagePersisted,
		ConversationID:    "conv-delivery",
		ConversationSeq:   seq,
		FanoutMode:        "WRITE_FANOUT",
		PermissionVersion: 1,
		MessageID:         messageID,
		SenderID:          "sender-1",
		PayloadJSON:       []byte(`{"text":"hello"}`),
	}
}

func ackCommand(receivedSeq int64) types.AckDeliveryCommand {
	return types.AckDeliveryCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-delivery",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conv-delivery",
		ReceivedSeq:    receivedSeq,
	}
}

func seedInbox(t *testing.T, ctx context.Context, pool *pgxpool.Pool, seq int64) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO user_inbox (
    tenant_id,
    user_id,
    conversation_id,
    conversation_seq,
    event_id,
    event_type,
    message_id,
    sender_id,
    payload_json,
    fanout_mode,
    permission_version
) VALUES ($1, 'user-1', 'conv-delivery', $2, $3, 'message.persisted.v1', $4, 'sender-1', '{}'::jsonb, 'WRITE_FANOUT', 1)
`, "tenant-delivery", seq, fmt.Sprintf("seed-event-%d", seq), fmt.Sprintf("seed-msg-%d", seq))
	if err != nil {
		t.Fatalf("seed inbox: %v", err)
	}
}

func assertDeliveryCursor(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int64) {
	t.Helper()
	var got int64
	err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM device_delivery_cursors
WHERE tenant_id = 'tenant-delivery'
  AND user_id = 'user-1'
  AND device_id = 'device-1'
  AND conversation_id = 'conv-delivery'
`).Scan(&got)
	if err != nil {
		t.Fatalf("read cursor: %v", err)
	}
	if got != want {
		t.Fatalf("expected cursor %d, got %d", want, got)
	}
}

func assertDeliveryOutboxCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventType string, want int) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM delivery_outbox
WHERE tenant_id = 'tenant-delivery'
  AND event_type = $1
`, eventType).Scan(&got)
	if err != nil {
		t.Fatalf("count outbox: %v", err)
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
	applyDeliveryMigration(t, ctx, pool)
	return pool
}

func applyDeliveryMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	root := findRepoRoot(t)
	migrationPath := filepath.Join(root, "migrations", "postgres", "delivery", "000001_delivery_core.sql")
	sqlBytes, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
}

func resetDeliveryTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    delivery_outbox,
    device_delivery_cursors,
    user_inbox,
    delivery_membership_projection,
    delivery_kafka_checkpoints
RESTART IDENTITY
`)
	if err != nil {
		t.Fatalf("reset delivery tables: %v", err)
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
