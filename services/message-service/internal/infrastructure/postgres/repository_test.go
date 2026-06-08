package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func TestMessageRepositoryAppendMessageIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	nextID := 0
	runID := time.Now().UnixNano()
	repo := NewMessageRepository(
		pool,
		WithClock(func() time.Time { return now }),
		WithIDGenerators(
			func() (types.MessageID, error) {
				nextID++
				return types.MessageID(fmt.Sprintf("msg-test-%d-%d", runID, nextID)), nil
			},
			func() (types.EventID, error) {
				return types.EventID(fmt.Sprintf("event-test-%d-%d", runID, nextID)), nil
			},
		),
	)

	tenantID := types.TenantID(fmt.Sprintf("tenant-it-%d", runID))
	input := testAppendInput(tenantID, "client-1", []byte(`{"text":"hello"}`))
	result, err := repo.AppendMessage(ctx, input)
	if err != nil {
		t.Fatalf("append message: %v", err)
	}
	if result.MessageID == "" || result.ConversationSeq != 1 || result.IdempotentReplay {
		t.Fatalf("unexpected first result: %+v", result)
	}

	replay, err := repo.AppendMessage(ctx, input)
	if err != nil {
		t.Fatalf("replay message: %v", err)
	}
	if !replay.IdempotentReplay || replay.MessageID != result.MessageID || replay.ConversationSeq != result.ConversationSeq {
		t.Fatalf("unexpected replay result: %+v", replay)
	}

	conflictInput := testAppendInput(tenantID, "client-1", []byte(`{"text":"changed"}`))
	_, err = repo.AppendMessage(ctx, conflictInput)
	if !errors.Is(err, types.ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	assertCount(t, ctx, pool, "message_log", input.Command.AuthContext.TenantID, 1)
	assertCount(t, ctx, pool, "conversation_timeline_events", input.Command.AuthContext.TenantID, 1)
	assertCount(t, ctx, pool, "message_outbox", input.Command.AuthContext.TenantID, 1)
	assertPersistedFacts(t, ctx, pool, input, result)
}

func TestMessageRepositoryAppendMessageConcurrentReplayDoesNotAdvanceSeq(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	repo := NewMessageRepository(
		pool,
		WithClock(func() time.Time { return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC) }),
	)
	runID := time.Now().UnixNano()
	tenantID := types.TenantID(fmt.Sprintf("tenant-it-concurrent-%d", runID))
	input := testAppendInput(tenantID, "client-concurrent", []byte(`{"text":"hello"}`))

	const workers = 8
	type appendOutcome struct {
		result domain.AppendMessageResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan appendOutcome, workers)
	for i := 0; i < workers; i++ {
		go func() {
			<-start
			result, err := repo.AppendMessage(ctx, input)
			outcomes <- appendOutcome{result: result, err: err}
		}()
	}
	close(start)

	var messageID types.MessageID
	replayCount := 0
	for i := 0; i < workers; i++ {
		outcome := <-outcomes
		if outcome.err != nil {
			t.Fatalf("append message concurrently: %v", outcome.err)
		}
		if outcome.result.ConversationSeq != 1 {
			t.Fatalf("unexpected conversation seq: %+v", outcome.result)
		}
		if messageID == "" {
			messageID = outcome.result.MessageID
		}
		if outcome.result.MessageID != messageID {
			t.Fatalf("unexpected message id: got %s want %s", outcome.result.MessageID, messageID)
		}
		if outcome.result.IdempotentReplay {
			replayCount++
		}
	}
	if replayCount != workers-1 {
		t.Fatalf("unexpected replay count: got %d want %d", replayCount, workers-1)
	}

	assertCount(t, ctx, pool, "message_log", tenantID, 1)
	assertCount(t, ctx, pool, "conversation_timeline_events", tenantID, 1)
	assertCount(t, ctx, pool, "message_outbox", tenantID, 1)
	assertCurrentSeq(t, ctx, pool, tenantID, input.Command.ConversationID, 1)
}

func openIntegrationPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("set NEXUSIM_PG_DSN to run PostgreSQL integration test")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	return pool
}

func applyMessageMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	migrationPath := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "message", "000001_message_core.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
}

func testAppendInput(tenant types.TenantID, clientMsgID types.ClientMsgID, payload []byte) domain.AppendMessageInput {
	return domain.AppendMessageInput{
		Command: types.SendMessageCommand{
			AuthContext: types.AuthContext{
				TenantID:  tenant,
				UserID:    "user-it",
				DeviceID:  "device-it",
				SessionID: "session-it",
				TraceID:   "trace-it",
				RequestID: "request-it",
			},
			ConversationID: "conversation-it",
			ClientMsgID:    clientMsgID,
			MessageType:    types.MessageTypeText,
			PayloadJSON:    payload,
			ReceivedAt:     time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
		},
		Permission: types.PermissionDecision{
			Allowed:           true,
			PermissionVersion: 1,
			Classification:    "INTERNAL",
		},
		Conversation: types.ConversationSendContext{
			MemberVersion:       1,
			PermissionVersion:   1,
			ConversationMode:    types.ConversationModeLocalRowLock,
			FanoutMode:          types.FanoutModeWriteFanout,
			FanoutPolicyVersion: 1,
			CurrentSeqShard:     "local",
		},
	}
}

func assertCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string, tenantID types.TenantID, want int64) {
	t.Helper()
	var got int64
	query := "SELECT count(*) FROM " + table + " WHERE tenant_id = $1"
	if err := pool.QueryRow(ctx, query, tenantID).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("unexpected %s count: got %d want %d", table, got, want)
	}
}

func assertPersistedFacts(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	input domain.AppendMessageInput,
	result domain.AppendMessageResult,
) {
	t.Helper()
	var (
		commandHash         string
		messageStatus       string
		messagePermission   int64
		messageClass        string
		timelineEventID     types.EventID
		timelineSeq         int64
		timelineMessageID   types.MessageID
		timelineFanoutMode  string
		timelinePolicyVer   int64
		timelinePermVer     int64
		timelineClass       string
		outboxEventID       types.EventID
		outboxVersion       int64
		outboxPartitionKey  string
		outboxProducer      string
		outboxStatus        string
		outboxPayloadString string
	)
	if err := pool.QueryRow(ctx, `
SELECT
    ml.command_hash,
    ml.status,
    ml.permission_version,
    ml.classification,
    te.event_id,
    te.seq,
    te.message_id,
    te.fanout_mode,
    te.fanout_policy_version,
    te.permission_version,
    te.classification,
    mo.event_id,
    mo.aggregate_version,
    mo.partition_key,
    mo.producer,
    mo.status,
    mo.payload_json::text
FROM message_log ml
JOIN conversation_timeline_events te
  ON te.tenant_id = ml.tenant_id
 AND te.conversation_id = ml.conversation_id
 AND te.seq = ml.conversation_seq
JOIN message_outbox mo
  ON mo.tenant_id = ml.tenant_id
 AND mo.conversation_id = ml.conversation_id
 AND mo.aggregate_version = ml.conversation_seq
WHERE ml.tenant_id = $1
  AND ml.message_id = $2
`,
		input.Command.AuthContext.TenantID,
		result.MessageID,
	).Scan(
		&commandHash,
		&messageStatus,
		&messagePermission,
		&messageClass,
		&timelineEventID,
		&timelineSeq,
		&timelineMessageID,
		&timelineFanoutMode,
		&timelinePolicyVer,
		&timelinePermVer,
		&timelineClass,
		&outboxEventID,
		&outboxVersion,
		&outboxPartitionKey,
		&outboxProducer,
		&outboxStatus,
		&outboxPayloadString,
	); err != nil {
		t.Fatalf("read persisted facts: %v", err)
	}

	if commandHash == "" || messageStatus != "NORMAL" {
		t.Fatalf("unexpected message facts: hash=%q status=%q", commandHash, messageStatus)
	}
	if messagePermission != input.Permission.PermissionVersion || messageClass != input.Permission.Classification {
		t.Fatalf("unexpected message policy facts: permission=%d class=%s", messagePermission, messageClass)
	}
	if timelineEventID != outboxEventID || timelineSeq != result.ConversationSeq || timelineMessageID != result.MessageID {
		t.Fatalf("timeline/outbox mismatch: timeline=%s outbox=%s seq=%d message=%s", timelineEventID, outboxEventID, timelineSeq, timelineMessageID)
	}
	if timelineFanoutMode != string(input.Conversation.FanoutMode) ||
		timelinePolicyVer != input.Conversation.FanoutPolicyVersion ||
		timelinePermVer != input.Permission.PermissionVersion ||
		timelineClass != input.Permission.Classification {
		t.Fatalf("unexpected timeline metadata")
	}
	if outboxVersion != result.ConversationSeq ||
		outboxPartitionKey != string(input.Command.AuthContext.TenantID)+":"+string(input.Command.ConversationID) ||
		outboxProducer != "message-service" ||
		outboxStatus != "PENDING" {
		t.Fatalf("unexpected outbox facts: version=%d partition=%s producer=%s status=%s", outboxVersion, outboxPartitionKey, outboxProducer, outboxStatus)
	}
	var outboxPayload map[string]any
	if err := json.Unmarshal([]byte(outboxPayloadString), &outboxPayload); err != nil {
		t.Fatalf("decode outbox payload: %v", err)
	}
	if outboxPayload["command_hash"] != commandHash ||
		outboxPayload["message_id"] != string(result.MessageID) ||
		int64(outboxPayload["conversation_seq"].(float64)) != result.ConversationSeq {
		t.Fatalf("unexpected outbox payload: %+v", outboxPayload)
	}
}

func assertCurrentSeq(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID types.TenantID,
	conversationID types.ConversationID,
	want int64,
) {
	t.Helper()
	var got int64
	if err := pool.QueryRow(ctx, `
SELECT current_seq
FROM conversation_seq
WHERE tenant_id = $1
  AND conversation_id = $2
`, tenantID, conversationID).Scan(&got); err != nil {
		t.Fatalf("get conversation seq: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected current_seq: got %d want %d", got, want)
	}
}
