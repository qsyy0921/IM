package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

func TestMessageRepositoryAppendAttachmentMessageIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	runID := time.Now().UnixNano()
	repo := NewMessageRepository(pool)
	tenantID := types.TenantID(fmt.Sprintf("tenant-it-image-%d", runID))
	input := testAppendInput(tenantID, "client-image-1", []byte(`{"caption":"hello","height":480,"width":640}`))
	input.Command.MessageType = types.MessageTypeImage
	input.Command.AttachmentIDs = []string{"image-2", "image-1"}

	result, err := repo.AppendMessage(ctx, input)
	if err != nil {
		t.Fatalf("append image message: %v", err)
	}
	if result.MessageID == "" || result.ConversationSeq != 1 {
		t.Fatalf("unexpected image append result: %+v", result)
	}
	assertPersistedFacts(t, ctx, pool, input, result)
}

func TestMessageRepositoryGetMessagePolicyContextIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	runID := time.Now().UnixNano()
	repo := NewMessageRepository(pool)
	tenantID := types.TenantID(fmt.Sprintf("tenant-policy-context-%d", runID))
	input := testAppendInput(tenantID, "client-policy-context", []byte(`{"text":"hello"}`))
	result, err := repo.AppendMessage(ctx, input)
	if err != nil {
		t.Fatalf("append message: %v", err)
	}

	policyContext, err := repo.GetMessagePolicyContext(ctx, tenantID, input.Command.ConversationID, result.MessageID)
	if err != nil {
		t.Fatalf("get message policy context: %v", err)
	}
	if policyContext.SenderUserID != input.Command.AuthContext.UserID {
		t.Fatalf("unexpected sender context: %+v", policyContext)
	}

	_, err = repo.GetMessagePolicyContext(ctx, tenantID, input.Command.ConversationID, "missing-message")
	if !errors.Is(err, types.ErrMessageNotFound) {
		t.Fatalf("expected message not found, got %v", err)
	}
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

func TestMessageRepositoryRevokeMessageIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	now := time.Date(2026, 6, 10, 1, 0, 0, 0, time.UTC)
	runID := time.Now().UnixNano()
	messageCounter := 0
	eventCounter := 0
	repo := NewMessageRepository(
		pool,
		WithClock(func() time.Time { return now }),
		WithIDGenerators(
			func() (types.MessageID, error) {
				messageCounter++
				return types.MessageID(fmt.Sprintf("msg-revoke-%d-%d", runID, messageCounter)), nil
			},
			func() (types.EventID, error) {
				eventCounter++
				return types.EventID(fmt.Sprintf("event-revoke-%d-%d", runID, eventCounter)), nil
			},
		),
	)
	tenantID := types.TenantID(fmt.Sprintf("tenant-it-revoke-%d", runID))
	appendInput := testAppendInput(tenantID, "client-revoke-source", []byte(`{"text":"hello"}`))
	appendResult, err := repo.AppendMessage(ctx, appendInput)
	if err != nil {
		t.Fatalf("append source message: %v", err)
	}

	revokeInput := testRevokeInput(appendInput, appendResult.MessageID, "revoke-key-1", "mistake")
	result, err := repo.RevokeMessage(ctx, revokeInput)
	if err != nil {
		t.Fatalf("revoke message: %v", err)
	}
	if result.MessageID != appendResult.MessageID ||
		result.ConversationSeq != 2 ||
		result.ChangeVersion != 1 ||
		result.IdempotentReplay {
		t.Fatalf("unexpected revoke result: %+v", result)
	}

	replay, err := repo.RevokeMessage(ctx, revokeInput)
	if err != nil {
		t.Fatalf("replay revoke: %v", err)
	}
	if !replay.IdempotentReplay ||
		replay.ConversationSeq != result.ConversationSeq ||
		replay.ChangeVersion != result.ChangeVersion {
		t.Fatalf("unexpected revoke replay: %+v", replay)
	}
	conflictInput := testRevokeInput(appendInput, appendResult.MessageID, "revoke-key-1", "different")
	_, err = repo.RevokeMessage(ctx, conflictInput)
	if !errors.Is(err, types.ErrIdempotencyConflict) {
		t.Fatalf("expected revoke idempotency conflict, got %v", err)
	}

	assertCount(t, ctx, pool, "message_log", tenantID, 1)
	assertCount(t, ctx, pool, "message_change_history", tenantID, 1)
	assertCount(t, ctx, pool, "conversation_timeline_events", tenantID, 2)
	assertCount(t, ctx, pool, "message_outbox", tenantID, 2)
	assertCurrentSeq(t, ctx, pool, tenantID, appendInput.Command.ConversationID, 2)
	assertRevokedFacts(t, ctx, pool, revokeInput, result)
}

func TestMessageRepositoryRevokeMessageRejectsNonSenderIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	runID := time.Now().UnixNano()
	messageCounter := 0
	eventCounter := 0
	repo := NewMessageRepository(
		pool,
		WithIDGenerators(
			func() (types.MessageID, error) {
				messageCounter++
				return types.MessageID(fmt.Sprintf("msg-revoke-nonsender-%d-%d", runID, messageCounter)), nil
			},
			func() (types.EventID, error) {
				eventCounter++
				return types.EventID(fmt.Sprintf("event-revoke-nonsender-%d-%d", runID, eventCounter)), nil
			},
		),
	)
	tenantID := types.TenantID(fmt.Sprintf("tenant-it-revoke-nonsender-%d", runID))
	appendInput := testAppendInput(tenantID, "client-revoke-nonsender", []byte(`{"text":"hello"}`))
	appendResult, err := repo.AppendMessage(ctx, appendInput)
	if err != nil {
		t.Fatalf("append source message: %v", err)
	}

	revokeInput := testRevokeInput(appendInput, appendResult.MessageID, "revoke-nonsender-key", "not mine")
	revokeInput.Command.AuthContext.UserID = "other-user"
	_, err = repo.RevokeMessage(ctx, revokeInput)
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
	assertCurrentSeq(t, ctx, pool, tenantID, appendInput.Command.ConversationID, 1)
	assertCount(t, ctx, pool, "message_change_history", tenantID, 0)

	var status string
	if err := pool.QueryRow(ctx, `
SELECT status
FROM message_log
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
`, tenantID, appendInput.Command.ConversationID, appendResult.MessageID).Scan(&status); err != nil {
		t.Fatalf("read message status: %v", err)
	}
	if status != "NORMAL" {
		t.Fatalf("expected message to remain NORMAL, got %s", status)
	}
}

func TestMessageRepositoryEditMessageIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	now := time.Date(2026, 6, 10, 2, 0, 0, 0, time.UTC)
	runID := time.Now().UnixNano()
	messageCounter := 0
	eventCounter := 0
	repo := NewMessageRepository(
		pool,
		WithClock(func() time.Time { return now }),
		WithIDGenerators(
			func() (types.MessageID, error) {
				messageCounter++
				return types.MessageID(fmt.Sprintf("msg-edit-%d-%d", runID, messageCounter)), nil
			},
			func() (types.EventID, error) {
				eventCounter++
				return types.EventID(fmt.Sprintf("event-edit-%d-%d", runID, eventCounter)), nil
			},
		),
	)
	tenantID := types.TenantID(fmt.Sprintf("tenant-it-edit-%d", runID))
	appendInput := testAppendInput(tenantID, "client-edit-source", []byte(`{"text":"hello"}`))
	appendResult, err := repo.AppendMessage(ctx, appendInput)
	if err != nil {
		t.Fatalf("append source message: %v", err)
	}

	editInput := testEditInput(appendInput, appendResult.MessageID, "edit-key-1", []byte(`{"text":"hello edited"}`), "typo")
	result, err := repo.EditMessage(ctx, editInput)
	if err != nil {
		t.Fatalf("edit message: %v", err)
	}
	if result.MessageID != appendResult.MessageID ||
		result.ConversationSeq != 2 ||
		result.ChangeVersion != 1 ||
		result.IdempotentReplay {
		t.Fatalf("unexpected edit result: %+v", result)
	}

	replay, err := repo.EditMessage(ctx, editInput)
	if err != nil {
		t.Fatalf("replay edit: %v", err)
	}
	if !replay.IdempotentReplay ||
		replay.ConversationSeq != result.ConversationSeq ||
		replay.ChangeVersion != result.ChangeVersion {
		t.Fatalf("unexpected edit replay: %+v", replay)
	}
	conflictInput := testEditInput(appendInput, appendResult.MessageID, "edit-key-1", []byte(`{"text":"different"}`), "typo")
	_, err = repo.EditMessage(ctx, conflictInput)
	if !errors.Is(err, types.ErrIdempotencyConflict) {
		t.Fatalf("expected edit idempotency conflict, got %v", err)
	}

	assertCount(t, ctx, pool, "message_log", tenantID, 1)
	assertCount(t, ctx, pool, "message_change_history", tenantID, 1)
	assertCount(t, ctx, pool, "conversation_timeline_events", tenantID, 2)
	assertCount(t, ctx, pool, "message_outbox", tenantID, 2)
	assertCurrentSeq(t, ctx, pool, tenantID, appendInput.Command.ConversationID, 2)
	assertEditedFacts(t, ctx, pool, editInput, result)
}

func TestMessageRepositoryEditMessageRejectsNonSenderIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	runID := time.Now().UnixNano()
	messageCounter := 0
	eventCounter := 0
	repo := NewMessageRepository(
		pool,
		WithIDGenerators(
			func() (types.MessageID, error) {
				messageCounter++
				return types.MessageID(fmt.Sprintf("msg-edit-nonsender-%d-%d", runID, messageCounter)), nil
			},
			func() (types.EventID, error) {
				eventCounter++
				return types.EventID(fmt.Sprintf("event-edit-nonsender-%d-%d", runID, eventCounter)), nil
			},
		),
	)
	tenantID := types.TenantID(fmt.Sprintf("tenant-it-edit-nonsender-%d", runID))
	appendInput := testAppendInput(tenantID, "client-edit-nonsender", []byte(`{"text":"hello"}`))
	appendResult, err := repo.AppendMessage(ctx, appendInput)
	if err != nil {
		t.Fatalf("append source message: %v", err)
	}

	editInput := testEditInput(appendInput, appendResult.MessageID, "edit-nonsender-key", []byte(`{"text":"not mine"}`), "not mine")
	editInput.Command.AuthContext.UserID = "other-user"
	_, err = repo.EditMessage(ctx, editInput)
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
	assertCurrentSeq(t, ctx, pool, tenantID, appendInput.Command.ConversationID, 1)
	assertCount(t, ctx, pool, "message_change_history", tenantID, 0)

	var status string
	var payload string
	if err := pool.QueryRow(ctx, `
SELECT status, payload_json::text
FROM message_log
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
`, tenantID, appendInput.Command.ConversationID, appendResult.MessageID).Scan(&status, &payload); err != nil {
		t.Fatalf("read message status: %v", err)
	}
	if status != "NORMAL" || payload != `{"text": "hello"}` {
		t.Fatalf("expected message unchanged, status=%s payload=%s", status, payload)
	}
}

func TestMessageRepositoryEditMessageAllowsNonSenderOwnershipOverrideIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	now := time.Date(2026, 6, 10, 2, 30, 0, 0, time.UTC)
	runID := time.Now().UnixNano()
	messageCounter := 0
	eventCounter := 0
	repo := NewMessageRepository(
		pool,
		WithClock(func() time.Time { return now }),
		WithIDGenerators(
			func() (types.MessageID, error) {
				messageCounter++
				return types.MessageID(fmt.Sprintf("msg-edit-override-%d-%d", runID, messageCounter)), nil
			},
			func() (types.EventID, error) {
				eventCounter++
				return types.EventID(fmt.Sprintf("event-edit-override-%d-%d", runID, eventCounter)), nil
			},
		),
	)
	tenantID := types.TenantID(fmt.Sprintf("tenant-it-edit-override-%d", runID))
	appendInput := testAppendInput(tenantID, "client-edit-override", []byte(`{"text":"hello"}`))
	appendResult, err := repo.AppendMessage(ctx, appendInput)
	if err != nil {
		t.Fatalf("append source message: %v", err)
	}

	editInput := testEditInput(appendInput, appendResult.MessageID, "edit-override-key", []byte(`{"text":"hello edited"}`), "moderation")
	editInput.Command.AuthContext.UserID = "admin-user"
	editInput.Permission.Classification = "MESSAGE_OWNERSHIP_ROLE_OVERRIDE"
	editInput.Permission.OwnershipOverride = true
	result, err := repo.EditMessage(ctx, editInput)
	if err != nil {
		t.Fatalf("edit message with ownership override: %v", err)
	}
	if result.MessageID != appendResult.MessageID ||
		result.ConversationSeq != 2 ||
		result.ChangeVersion != 1 ||
		result.IdempotentReplay {
		t.Fatalf("unexpected edit override result: %+v", result)
	}
	assertCount(t, ctx, pool, "message_change_history", tenantID, 1)
	assertCount(t, ctx, pool, "conversation_timeline_events", tenantID, 2)
	assertCount(t, ctx, pool, "message_outbox", tenantID, 2)
	assertCurrentSeq(t, ctx, pool, tenantID, appendInput.Command.ConversationID, 2)
	assertEditedFacts(t, ctx, pool, editInput, result)
}

func TestMessageRepositoryDeleteMessageIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	now := time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)
	runID := time.Now().UnixNano()
	messageCounter := 0
	eventCounter := 0
	repo := NewMessageRepository(
		pool,
		WithClock(func() time.Time { return now }),
		WithIDGenerators(
			func() (types.MessageID, error) {
				messageCounter++
				return types.MessageID(fmt.Sprintf("msg-delete-%d-%d", runID, messageCounter)), nil
			},
			func() (types.EventID, error) {
				eventCounter++
				return types.EventID(fmt.Sprintf("event-delete-%d-%d", runID, eventCounter)), nil
			},
		),
	)
	tenantID := types.TenantID(fmt.Sprintf("tenant-it-delete-%d", runID))
	appendInput := testAppendInput(tenantID, "client-delete-source", []byte(`{"text":"hello"}`))
	appendResult, err := repo.AppendMessage(ctx, appendInput)
	if err != nil {
		t.Fatalf("append source message: %v", err)
	}

	deleteInput := testDeleteInput(appendInput, appendResult.MessageID, "delete-key-1", types.DeleteScopeConversationView, "cleanup")
	result, err := repo.DeleteMessage(ctx, deleteInput)
	if err != nil {
		t.Fatalf("delete message: %v", err)
	}
	if result.MessageID != appendResult.MessageID ||
		result.ConversationSeq != 2 ||
		result.ChangeVersion != 1 ||
		result.IdempotentReplay {
		t.Fatalf("unexpected delete result: %+v", result)
	}

	replay, err := repo.DeleteMessage(ctx, deleteInput)
	if err != nil {
		t.Fatalf("replay delete: %v", err)
	}
	if !replay.IdempotentReplay ||
		replay.ConversationSeq != result.ConversationSeq ||
		replay.ChangeVersion != result.ChangeVersion {
		t.Fatalf("unexpected delete replay: %+v", replay)
	}
	conflictInput := testDeleteInput(appendInput, appendResult.MessageID, "delete-key-1", types.DeleteScopeConversationView, "different")
	_, err = repo.DeleteMessage(ctx, conflictInput)
	if !errors.Is(err, types.ErrIdempotencyConflict) {
		t.Fatalf("expected delete idempotency conflict, got %v", err)
	}

	assertCount(t, ctx, pool, "message_log", tenantID, 1)
	assertCount(t, ctx, pool, "message_change_history", tenantID, 1)
	assertCount(t, ctx, pool, "conversation_timeline_events", tenantID, 2)
	assertCount(t, ctx, pool, "message_outbox", tenantID, 2)
	assertCurrentSeq(t, ctx, pool, tenantID, appendInput.Command.ConversationID, 2)
	assertDeletedFacts(t, ctx, pool, deleteInput, result)
}

func TestMessageRepositoryDeleteMessageRejectsNonSenderIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	runID := time.Now().UnixNano()
	messageCounter := 0
	eventCounter := 0
	repo := NewMessageRepository(
		pool,
		WithIDGenerators(
			func() (types.MessageID, error) {
				messageCounter++
				return types.MessageID(fmt.Sprintf("msg-delete-nonsender-%d-%d", runID, messageCounter)), nil
			},
			func() (types.EventID, error) {
				eventCounter++
				return types.EventID(fmt.Sprintf("event-delete-nonsender-%d-%d", runID, eventCounter)), nil
			},
		),
	)
	tenantID := types.TenantID(fmt.Sprintf("tenant-it-delete-nonsender-%d", runID))
	appendInput := testAppendInput(tenantID, "client-delete-nonsender", []byte(`{"text":"hello"}`))
	appendResult, err := repo.AppendMessage(ctx, appendInput)
	if err != nil {
		t.Fatalf("append source message: %v", err)
	}

	deleteInput := testDeleteInput(appendInput, appendResult.MessageID, "delete-nonsender-key", types.DeleteScopeConversationView, "not mine")
	deleteInput.Command.AuthContext.UserID = "other-user"
	_, err = repo.DeleteMessage(ctx, deleteInput)
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
	assertCurrentSeq(t, ctx, pool, tenantID, appendInput.Command.ConversationID, 1)
	assertCount(t, ctx, pool, "message_change_history", tenantID, 0)

	var status string
	if err := pool.QueryRow(ctx, `
SELECT status
FROM message_log
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
`, tenantID, appendInput.Command.ConversationID, appendResult.MessageID).Scan(&status); err != nil {
		t.Fatalf("read message status: %v", err)
	}
	if status != "NORMAL" {
		t.Fatalf("expected message unchanged, status=%s", status)
	}
}

func TestMessageRepositoryBackpressureRejectsWhenPoolSaturated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("set NEXUSIM_PG_DSN to run PostgreSQL integration test")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse postgres config: %v", err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire saturation connection: %v", err)
	}
	defer conn.Release()

	repo := NewMessageRepository(pool, WithBackpressure(BackpressureConfig{Enabled: true}))
	_, err = repo.AppendMessage(ctx, testAppendInput(types.TenantID("tenant-backpressure"), "client-backpressure", []byte(`{"text":"hello"}`)))
	if !errors.Is(err, types.ErrServiceOverloaded) {
		t.Fatalf("expected service overloaded, got %v", err)
	}
}

func testEditInput(
	appendInput domain.AppendMessageInput,
	messageID types.MessageID,
	idempotencyKey string,
	payload []byte,
	reason string,
) domain.EditMessageInput {
	return domain.EditMessageInput{
		Command: types.EditMessageCommand{
			AuthContext:    appendInput.Command.AuthContext,
			ConversationID: appendInput.Command.ConversationID,
			MessageID:      messageID,
			IdempotencyKey: idempotencyKey,
			PayloadJSON:    payload,
			Reason:         reason,
			ReceivedAt:     time.Date(2026, 6, 10, 2, 0, 0, 0, time.UTC),
		},
		Permission:   appendInput.Permission,
		Conversation: appendInput.Conversation,
	}
}

func testRevokeInput(
	appendInput domain.AppendMessageInput,
	messageID types.MessageID,
	idempotencyKey string,
	reason string,
) domain.RevokeMessageInput {
	return domain.RevokeMessageInput{
		Command: types.RevokeMessageCommand{
			AuthContext:    appendInput.Command.AuthContext,
			ConversationID: appendInput.Command.ConversationID,
			MessageID:      messageID,
			IdempotencyKey: idempotencyKey,
			Reason:         reason,
			ReceivedAt:     time.Date(2026, 6, 10, 1, 0, 0, 0, time.UTC),
		},
		Permission:   appendInput.Permission,
		Conversation: appendInput.Conversation,
	}
}

func testDeleteInput(
	appendInput domain.AppendMessageInput,
	messageID types.MessageID,
	idempotencyKey string,
	deleteScope types.DeleteScope,
	reason string,
) domain.DeleteMessageInput {
	return domain.DeleteMessageInput{
		Command: types.DeleteMessageCommand{
			AuthContext:    appendInput.Command.AuthContext,
			ConversationID: appendInput.Command.ConversationID,
			MessageID:      messageID,
			IdempotencyKey: idempotencyKey,
			DeleteScope:    deleteScope,
			Reason:         reason,
			ReceivedAt:     time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC),
		},
		Permission:   appendInput.Permission,
		Conversation: appendInput.Conversation,
	}
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
	migrationDir := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "message")
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		t.Fatalf("read migration dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		migrationSQL, err := os.ReadFile(filepath.Join(migrationDir, entry.Name()))
		if err != nil {
			t.Fatalf("read migration %s: %v", entry.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(migrationSQL)); err != nil {
			t.Fatalf("apply migration %s: %v", entry.Name(), err)
		}
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

func assertEditedFacts(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	input domain.EditMessageInput,
	result domain.MessageChangeResult,
) {
	t.Helper()
	var (
		status           string
		payloadString    string
		editedAt         *time.Time
		changeType       string
		beforePayload    string
		afterPayload     string
		beforeStatus     string
		afterStatus      string
		changeVersion    int32
		timelineEventID  types.EventID
		timelineType     string
		timelineSeq      int64
		outboxEventID    types.EventID
		outboxType       string
		outboxVersion    int64
		outboxPayloadRaw string
	)
	if err := pool.QueryRow(ctx, `
SELECT
    ml.status,
    ml.payload_json::text,
    ml.edited_at,
    mch.change_type,
    mch.before_payload_json::text,
    mch.after_payload_json::text,
    mch.before_status,
    mch.after_status,
    mch.change_version,
    te.event_id,
    te.event_type,
    te.seq,
    mo.event_id,
    mo.event_type,
    mo.aggregate_version,
    mo.payload_json::text
FROM message_log ml
JOIN message_change_history mch
  ON mch.tenant_id = ml.tenant_id
 AND mch.conversation_id = ml.conversation_id
 AND mch.message_id = ml.message_id
JOIN conversation_timeline_events te
  ON te.tenant_id = ml.tenant_id
 AND te.conversation_id = ml.conversation_id
 AND te.seq = $4
JOIN message_outbox mo
  ON mo.tenant_id = te.tenant_id
 AND mo.conversation_id = te.conversation_id
 AND mo.aggregate_version = te.seq
WHERE ml.tenant_id = $1
  AND ml.conversation_id = $2
  AND ml.message_id = $3
`,
		input.Command.AuthContext.TenantID,
		input.Command.ConversationID,
		input.Command.MessageID,
		result.ConversationSeq,
	).Scan(
		&status,
		&payloadString,
		&editedAt,
		&changeType,
		&beforePayload,
		&afterPayload,
		&beforeStatus,
		&afterStatus,
		&changeVersion,
		&timelineEventID,
		&timelineType,
		&timelineSeq,
		&outboxEventID,
		&outboxType,
		&outboxVersion,
		&outboxPayloadRaw,
	); err != nil {
		t.Fatalf("read edited facts: %v", err)
	}
	if status != "EDITED" || editedAt == nil || payloadString != `{"text": "hello edited"}` {
		t.Fatalf("unexpected message edit status=%s editedAt=%v payload=%s", status, editedAt, payloadString)
	}
	if changeType != "EDIT" ||
		beforePayload != `{"text": "hello"}` ||
		afterPayload != `{"text": "hello edited"}` ||
		beforeStatus != "NORMAL" ||
		afterStatus != "EDITED" ||
		changeVersion != result.ChangeVersion {
		t.Fatalf("unexpected change history type=%s beforePayload=%s afterPayload=%s before=%s after=%s version=%d", changeType, beforePayload, afterPayload, beforeStatus, afterStatus, changeVersion)
	}
	if timelineEventID != outboxEventID ||
		timelineType != string(types.TimelineEventMessageEdited) ||
		outboxType != string(types.TimelineEventMessageEdited) ||
		timelineSeq != result.ConversationSeq ||
		outboxVersion != result.ConversationSeq {
		t.Fatalf("unexpected edit timeline/outbox event timeline=%s/%s seq=%d outbox=%s/%s version=%d", timelineEventID, timelineType, timelineSeq, outboxEventID, outboxType, outboxVersion)
	}
	var outboxPayload map[string]any
	if err := json.Unmarshal([]byte(outboxPayloadRaw), &outboxPayload); err != nil {
		t.Fatalf("decode edit outbox payload: %v", err)
	}
	if outboxPayload["message_id"] != string(input.Command.MessageID) ||
		outboxPayload["edited_by"] != string(input.Command.AuthContext.UserID) ||
		int64(outboxPayload["conversation_seq"].(float64)) != result.ConversationSeq {
		t.Fatalf("unexpected edit payload: %+v", outboxPayload)
	}
}

func assertRevokedFacts(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	input domain.RevokeMessageInput,
	result domain.MessageChangeResult,
) {
	t.Helper()
	var (
		status           string
		revokedAt        *time.Time
		changeType       string
		beforeStatus     string
		afterStatus      string
		changeVersion    int32
		timelineEventID  types.EventID
		timelineType     string
		timelineSeq      int64
		outboxEventID    types.EventID
		outboxType       string
		outboxVersion    int64
		outboxPayloadRaw string
	)
	if err := pool.QueryRow(ctx, `
SELECT
    ml.status,
    ml.revoked_at,
    mch.change_type,
    mch.before_status,
    mch.after_status,
    mch.change_version,
    te.event_id,
    te.event_type,
    te.seq,
    mo.event_id,
    mo.event_type,
    mo.aggregate_version,
    mo.payload_json::text
FROM message_log ml
JOIN message_change_history mch
  ON mch.tenant_id = ml.tenant_id
 AND mch.conversation_id = ml.conversation_id
 AND mch.message_id = ml.message_id
JOIN conversation_timeline_events te
  ON te.tenant_id = ml.tenant_id
 AND te.conversation_id = ml.conversation_id
 AND te.seq = $4
JOIN message_outbox mo
  ON mo.tenant_id = te.tenant_id
 AND mo.conversation_id = te.conversation_id
 AND mo.aggregate_version = te.seq
WHERE ml.tenant_id = $1
  AND ml.conversation_id = $2
  AND ml.message_id = $3
`,
		input.Command.AuthContext.TenantID,
		input.Command.ConversationID,
		input.Command.MessageID,
		result.ConversationSeq,
	).Scan(
		&status,
		&revokedAt,
		&changeType,
		&beforeStatus,
		&afterStatus,
		&changeVersion,
		&timelineEventID,
		&timelineType,
		&timelineSeq,
		&outboxEventID,
		&outboxType,
		&outboxVersion,
		&outboxPayloadRaw,
	); err != nil {
		t.Fatalf("read revoked facts: %v", err)
	}
	if status != "REVOKED" || revokedAt == nil {
		t.Fatalf("unexpected message revoke status=%s revokedAt=%v", status, revokedAt)
	}
	if changeType != "REVOKE" || beforeStatus != "NORMAL" || afterStatus != "REVOKED" || changeVersion != result.ChangeVersion {
		t.Fatalf("unexpected change history type=%s before=%s after=%s version=%d", changeType, beforeStatus, afterStatus, changeVersion)
	}
	if timelineEventID != outboxEventID ||
		timelineType != string(types.TimelineEventMessageRevoked) ||
		outboxType != string(types.TimelineEventMessageRevoked) ||
		timelineSeq != result.ConversationSeq ||
		outboxVersion != result.ConversationSeq {
		t.Fatalf("unexpected revoke timeline/outbox event timeline=%s/%s seq=%d outbox=%s/%s version=%d", timelineEventID, timelineType, timelineSeq, outboxEventID, outboxType, outboxVersion)
	}
	var outboxPayload map[string]any
	if err := json.Unmarshal([]byte(outboxPayloadRaw), &outboxPayload); err != nil {
		t.Fatalf("decode revoke outbox payload: %v", err)
	}
	if outboxPayload["message_id"] != string(input.Command.MessageID) ||
		outboxPayload["revoked_by"] != string(input.Command.AuthContext.UserID) ||
		int64(outboxPayload["conversation_seq"].(float64)) != result.ConversationSeq {
		t.Fatalf("unexpected revoke payload: %+v", outboxPayload)
	}
}

func assertDeletedFacts(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	input domain.DeleteMessageInput,
	result domain.MessageChangeResult,
) {
	t.Helper()
	var (
		status           string
		deletedAt        *time.Time
		changeType       string
		beforeStatus     string
		afterStatus      string
		changeVersion    int32
		timelineEventID  types.EventID
		timelineType     string
		timelineSeq      int64
		outboxEventID    types.EventID
		outboxType       string
		outboxVersion    int64
		outboxPayloadRaw string
	)
	if err := pool.QueryRow(ctx, `
SELECT
    ml.status,
    ml.deleted_at,
    mch.change_type,
    mch.before_status,
    mch.after_status,
    mch.change_version,
    te.event_id,
    te.event_type,
    te.seq,
    mo.event_id,
    mo.event_type,
    mo.aggregate_version,
    mo.payload_json::text
FROM message_log ml
JOIN message_change_history mch
  ON mch.tenant_id = ml.tenant_id
 AND mch.conversation_id = ml.conversation_id
 AND mch.message_id = ml.message_id
JOIN conversation_timeline_events te
  ON te.tenant_id = ml.tenant_id
 AND te.conversation_id = ml.conversation_id
 AND te.seq = $4
JOIN message_outbox mo
  ON mo.tenant_id = te.tenant_id
 AND mo.conversation_id = te.conversation_id
 AND mo.aggregate_version = te.seq
WHERE ml.tenant_id = $1
  AND ml.conversation_id = $2
  AND ml.message_id = $3
`,
		input.Command.AuthContext.TenantID,
		input.Command.ConversationID,
		input.Command.MessageID,
		result.ConversationSeq,
	).Scan(
		&status,
		&deletedAt,
		&changeType,
		&beforeStatus,
		&afterStatus,
		&changeVersion,
		&timelineEventID,
		&timelineType,
		&timelineSeq,
		&outboxEventID,
		&outboxType,
		&outboxVersion,
		&outboxPayloadRaw,
	); err != nil {
		t.Fatalf("read deleted facts: %v", err)
	}
	if status != "DELETED" || deletedAt == nil {
		t.Fatalf("unexpected message delete status=%s deletedAt=%v", status, deletedAt)
	}
	if changeType != "DELETE" || beforeStatus != "NORMAL" || afterStatus != "DELETED" || changeVersion != result.ChangeVersion {
		t.Fatalf("unexpected change history type=%s before=%s after=%s version=%d", changeType, beforeStatus, afterStatus, changeVersion)
	}
	if timelineEventID != outboxEventID ||
		timelineType != string(types.TimelineEventMessageDeleted) ||
		outboxType != string(types.TimelineEventMessageDeleted) ||
		timelineSeq != result.ConversationSeq ||
		outboxVersion != result.ConversationSeq {
		t.Fatalf("unexpected delete timeline/outbox event timeline=%s/%s seq=%d outbox=%s/%s version=%d", timelineEventID, timelineType, timelineSeq, outboxEventID, outboxType, outboxVersion)
	}
	var outboxPayload map[string]any
	if err := json.Unmarshal([]byte(outboxPayloadRaw), &outboxPayload); err != nil {
		t.Fatalf("decode delete outbox payload: %v", err)
	}
	if outboxPayload["message_id"] != string(input.Command.MessageID) ||
		outboxPayload["deleted_by"] != string(input.Command.AuthContext.UserID) ||
		outboxPayload["delete_scope"] != string(types.DeleteScopeConversationView) ||
		int64(outboxPayload["conversation_seq"].(float64)) != result.ConversationSeq {
		t.Fatalf("unexpected delete payload: %+v", outboxPayload)
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
		messageType         string
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
    ml.message_type,
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
		&messageType,
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
	if messageType != string(input.Command.MessageType) {
		t.Fatalf("unexpected message type: got=%s want=%s", messageType, input.Command.MessageType)
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
		int64(outboxPayload["conversation_seq"].(float64)) != result.ConversationSeq ||
		outboxPayload["message_type"] != string(input.Command.MessageType) {
		t.Fatalf("unexpected outbox payload: %+v", outboxPayload)
	}
	if len(input.Command.AttachmentIDs) > 0 {
		attachments, ok := outboxPayload["attachment_ids"].([]any)
		if !ok || len(attachments) != len(input.Command.AttachmentIDs) {
			t.Fatalf("unexpected attachment payload: %+v", outboxPayload["attachment_ids"])
		}
		expected := append([]string(nil), input.Command.AttachmentIDs...)
		sort.Strings(expected)
		for index, want := range expected {
			if attachments[index] != want {
				t.Fatalf("unexpected attachment payload: got=%+v want=%+v", attachments, expected)
			}
		}
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
