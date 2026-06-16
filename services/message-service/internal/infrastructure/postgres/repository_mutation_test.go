package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

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

func TestMessageRepositoryDeleteMessageComplianceRedactsPayloadIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	now := time.Date(2026, 6, 10, 3, 30, 0, 0, time.UTC)
	runID := time.Now().UnixNano()
	messageCounter := 0
	eventCounter := 0
	repo := NewMessageRepository(
		pool,
		WithClock(func() time.Time { return now }),
		WithIDGenerators(
			func() (types.MessageID, error) {
				messageCounter++
				return types.MessageID(fmt.Sprintf("msg-delete-compliance-%d-%d", runID, messageCounter)), nil
			},
			func() (types.EventID, error) {
				eventCounter++
				return types.EventID(fmt.Sprintf("event-delete-compliance-%d-%d", runID, eventCounter)), nil
			},
		),
	)
	tenantID := types.TenantID(fmt.Sprintf("tenant-it-delete-compliance-%d", runID))
	appendInput := testAppendInput(tenantID, "client-delete-compliance-source", []byte(`{"text":"secret compliance payload"}`))
	appendResult, err := repo.AppendMessage(ctx, appendInput)
	if err != nil {
		t.Fatalf("append source message: %v", err)
	}

	deleteInput := testDeleteInput(appendInput, appendResult.MessageID, "delete-compliance-key-1", types.DeleteScopeCompliance, "legal retention cleanup")
	deleteInput.Command.AuthContext.UserID = "compliance-admin"
	deleteInput.Permission.OwnershipOverride = true
	deleteInput.Permission.Classification = "COMPLIANCE_RETENTION"
	result, err := repo.DeleteMessage(ctx, deleteInput)
	if err != nil {
		t.Fatalf("compliance delete message: %v", err)
	}
	if result.MessageID != appendResult.MessageID ||
		result.ConversationSeq != 2 ||
		result.ChangeVersion != 1 ||
		result.IdempotentReplay {
		t.Fatalf("unexpected compliance delete result: %+v", result)
	}

	assertDeletedFacts(t, ctx, pool, deleteInput, result)
	var currentPayload, beforePayload, afterPayload, outboxPayload string
	if err := pool.QueryRow(ctx, `
SELECT
    ml.payload_json::text,
    mch.before_payload_json::text,
    COALESCE(mch.after_payload_json::text, ''),
    mo.payload_json::text
FROM message_log ml
JOIN message_change_history mch
  ON mch.tenant_id = ml.tenant_id
 AND mch.conversation_id = ml.conversation_id
 AND mch.message_id = ml.message_id
JOIN message_outbox mo
  ON mo.tenant_id = ml.tenant_id
 AND mo.conversation_id = ml.conversation_id
 AND mo.aggregate_version = $4
WHERE ml.tenant_id = $1
  AND ml.conversation_id = $2
  AND ml.message_id = $3
`, tenantID, appendInput.Command.ConversationID, appendResult.MessageID, result.ConversationSeq).Scan(&currentPayload, &beforePayload, &afterPayload, &outboxPayload); err != nil {
		t.Fatalf("read compliance delete payload facts: %v", err)
	}
	for label, payload := range map[string]string{
		"current": currentPayload,
		"before":  beforePayload,
		"after":   afterPayload,
		"outbox":  outboxPayload,
	} {
		if strings.Contains(payload, "secret compliance payload") || strings.Contains(payload, "legal retention cleanup") {
			t.Fatalf("%s payload leaked raw content or reason: %s", label, payload)
		}
	}
	if !strings.Contains(currentPayload, `"redacted": true`) ||
		!strings.Contains(beforePayload, `"redacted": true`) ||
		!strings.Contains(afterPayload, `"redacted": true`) ||
		!strings.Contains(outboxPayload, `"delete_scope": "COMPLIANCE_RETENTION"`) {
		t.Fatalf("unexpected redaction payloads current=%s before=%s after=%s outbox=%s", currentPayload, beforePayload, afterPayload, outboxPayload)
	}
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
