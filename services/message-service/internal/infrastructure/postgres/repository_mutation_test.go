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
	deleteInput.Command.ComplianceApprovalID = fmt.Sprintf("approval-compliance-%d", runID)
	deleteInput.Command.ExternalProofRef = fmt.Sprintf("proof://compliance/%d", runID)
	deleteInput.Permission.OwnershipOverride = true
	deleteInput.Permission.Classification = "COMPLIANCE_RETENTION"
	if _, err := repo.RegisterComplianceExternalProof(ctx, MessageComplianceExternalProofMutationOptions{
		TenantID:         string(tenantID),
		ExternalProofRef: deleteInput.Command.ExternalProofRef,
		Provider:         "legal-proof-system",
		ProofHash:        fmt.Sprintf("sha256:proof-%d", runID),
		OperatorID:       "legal-ops",
		Now:              now,
	}); err != nil {
		t.Fatalf("register compliance proof: %v", err)
	}
	if _, err := repo.ApproveComplianceDelete(ctx, MessageComplianceDeleteApprovalMutationOptions{
		TenantID:         string(tenantID),
		ConversationID:   string(appendInput.Command.ConversationID),
		MessageID:        string(appendResult.MessageID),
		ApprovalID:       deleteInput.Command.ComplianceApprovalID,
		ExternalProofRef: deleteInput.Command.ExternalProofRef,
		OperatorID:       "legal-approver",
		Reason:           "approval reason with token=secret-token",
		Now:              now,
	}); err != nil {
		t.Fatalf("approve compliance delete: %v", err)
	}
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
	approvalRows, err := repo.AuditComplianceDeleteApprovals(ctx, MessageComplianceDeleteApprovalAuditOptions{
		TenantID:   string(tenantID),
		ApprovalID: deleteInput.Command.ComplianceApprovalID,
	})
	if err != nil {
		t.Fatalf("audit compliance approval: %v", err)
	}
	if len(approvalRows) != 1 ||
		approvalRows[0].Status != MessageComplianceApprovalStatusConsumed ||
		approvalRows[0].ConsumedEventID == "" ||
		approvalRows[0].ConsumedBy != "compliance-admin" {
		t.Fatalf("unexpected consumed compliance approval rows: %+v", approvalRows)
	}
	_, err = repo.ApproveComplianceDelete(ctx, MessageComplianceDeleteApprovalMutationOptions{
		TenantID:         string(tenantID),
		ConversationID:   string(appendInput.Command.ConversationID),
		MessageID:        string(appendResult.MessageID),
		ApprovalID:       deleteInput.Command.ComplianceApprovalID,
		ExternalProofRef: deleteInput.Command.ExternalProofRef,
		OperatorID:       "legal-approver",
		Reason:           "attempt to resurrect consumed approval",
		Now:              now.Add(time.Minute),
	})
	if !errors.Is(err, types.ErrInvalidMessageState) {
		t.Fatalf("expected consumed approval re-approve to fail, got %v", err)
	}
	approvalRows, err = repo.AuditComplianceDeleteApprovals(ctx, MessageComplianceDeleteApprovalAuditOptions{
		TenantID:   string(tenantID),
		ApprovalID: deleteInput.Command.ComplianceApprovalID,
	})
	if err != nil {
		t.Fatalf("audit compliance approval after failed re-approve: %v", err)
	}
	if len(approvalRows) != 1 || approvalRows[0].Status != MessageComplianceApprovalStatusConsumed {
		t.Fatalf("consumed compliance approval should remain consumed, got %+v", approvalRows)
	}
}

func TestMessageRepositoryDeleteMessageBlockedByLegalHoldIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	now := time.Date(2026, 6, 10, 3, 45, 0, 0, time.UTC)
	runID := time.Now().UnixNano()
	messageCounter := 0
	eventCounter := 0
	repo := NewMessageRepository(
		pool,
		WithClock(func() time.Time { return now }),
		WithIDGenerators(
			func() (types.MessageID, error) {
				messageCounter++
				return types.MessageID(fmt.Sprintf("msg-delete-hold-%d-%d", runID, messageCounter)), nil
			},
			func() (types.EventID, error) {
				eventCounter++
				return types.EventID(fmt.Sprintf("event-delete-hold-%d-%d", runID, eventCounter)), nil
			},
		),
	)
	tenantID := types.TenantID(fmt.Sprintf("tenant-it-delete-hold-%d", runID))
	appendInput := testAppendInput(tenantID, "client-delete-hold-source", []byte(`{"text":"legal hold payload"}`))
	appendResult, err := repo.AppendMessage(ctx, appendInput)
	if err != nil {
		t.Fatalf("append source message: %v", err)
	}
	_, err = repo.SetMessageLegalHold(ctx, MessageLegalHoldMutationOptions{
		TenantID:       string(tenantID),
		ConversationID: string(appendInput.Command.ConversationID),
		MessageID:      string(appendResult.MessageID),
		HoldID:         fmt.Sprintf("hold-%d", runID),
		OperatorID:     "legal-ops",
		Reason:         "do not leak legal reason with token=secret-token",
		Now:            now,
	})
	if err != nil {
		t.Fatalf("set legal hold: %v", err)
	}

	deleteInput := testDeleteInput(appendInput, appendResult.MessageID, "delete-held-key-1", types.DeleteScopeCompliance, "legal retention cleanup")
	deleteInput.Command.AuthContext.UserID = "compliance-admin"
	deleteInput.Permission.OwnershipOverride = true
	deleteInput.Permission.Classification = "COMPLIANCE_RETENTION"
	_, err = repo.DeleteMessage(ctx, deleteInput)
	if !errors.Is(err, types.ErrInvalidMessageState) {
		t.Fatalf("expected invalid message state for held message, got %v", err)
	}

	assertCurrentSeq(t, ctx, pool, tenantID, appendInput.Command.ConversationID, 1)
	assertCount(t, ctx, pool, "message_change_history", tenantID, 0)
	assertCount(t, ctx, pool, "conversation_timeline_events", tenantID, 1)
	assertCount(t, ctx, pool, "message_outbox", tenantID, 1)
	var status, payload string
	if err := pool.QueryRow(ctx, `
SELECT status, payload_json::text
FROM message_log
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
`, tenantID, appendInput.Command.ConversationID, appendResult.MessageID).Scan(&status, &payload); err != nil {
		t.Fatalf("read held message: %v", err)
	}
	if status != "NORMAL" || !strings.Contains(payload, "legal hold payload") {
		t.Fatalf("held message should remain unchanged, status=%s payload=%s", status, payload)
	}
}

func TestMessageRepositoryLegalHoldSetReleaseAuditIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	now := time.Date(2026, 6, 10, 3, 50, 0, 0, time.UTC)
	runID := time.Now().UnixNano()
	repo := NewMessageRepository(
		pool,
		WithClock(func() time.Time { return now }),
		WithIDGenerators(
			func() (types.MessageID, error) {
				return types.MessageID(fmt.Sprintf("msg-legal-hold-%d", runID)), nil
			},
			func() (types.EventID, error) {
				return types.EventID(fmt.Sprintf("event-legal-hold-%d", runID)), nil
			},
		),
	)
	tenantID := types.TenantID(fmt.Sprintf("tenant-it-legal-hold-%d", runID))
	appendInput := testAppendInput(tenantID, "client-legal-hold", []byte(`{"text":"hold me"}`))
	appendResult, err := repo.AppendMessage(ctx, appendInput)
	if err != nil {
		t.Fatalf("append source message: %v", err)
	}
	holdID := fmt.Sprintf("hold-set-release-%d", runID)
	setResult, err := repo.SetMessageLegalHold(ctx, MessageLegalHoldMutationOptions{
		TenantID:       string(tenantID),
		ConversationID: string(appendInput.Command.ConversationID),
		MessageID:      string(appendResult.MessageID),
		HoldID:         holdID,
		OperatorID:     "legal-ops",
		Reason:         "legal discovery",
		Now:            now,
	})
	if err != nil {
		t.Fatalf("set legal hold: %v", err)
	}
	if setResult.Status != MessageLegalHoldStatusActive || !setResult.ReasonPresent {
		t.Fatalf("unexpected set result: %+v", setResult)
	}
	activeRows, err := repo.AuditMessageLegalHolds(ctx, MessageLegalHoldAuditOptions{
		TenantID: string(tenantID),
		Status:   MessageLegalHoldStatusActive,
	})
	if err != nil {
		t.Fatalf("audit active legal hold: %v", err)
	}
	if len(activeRows) != 1 || activeRows[0].HoldID != holdID {
		t.Fatalf("unexpected active legal hold audit rows: %+v", activeRows)
	}

	released, err := repo.ReleaseMessageLegalHold(ctx, MessageLegalHoldMutationOptions{
		TenantID:   string(tenantID),
		HoldID:     holdID,
		OperatorID: "legal-ops",
		Now:        now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("release legal hold: %v", err)
	}
	if released.Status != MessageLegalHoldStatusReleased || released.ReleasedAt == nil || released.ReleasedBy != "legal-ops" {
		t.Fatalf("unexpected release result: %+v", released)
	}
	releasedRows, err := repo.AuditMessageLegalHolds(ctx, MessageLegalHoldAuditOptions{
		TenantID: string(tenantID),
		Status:   MessageLegalHoldStatusReleased,
	})
	if err != nil {
		t.Fatalf("audit released legal hold: %v", err)
	}
	if len(releasedRows) != 1 || releasedRows[0].HoldID != holdID {
		t.Fatalf("unexpected released legal hold audit rows: %+v", releasedRows)
	}
}

func TestMessageRepositoryComplianceDeleteApprovalSetCancelAuditIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	now := time.Date(2026, 6, 10, 3, 55, 0, 0, time.UTC)
	runID := time.Now().UnixNano()
	repo := NewMessageRepository(
		pool,
		WithClock(func() time.Time { return now }),
		WithIDGenerators(
			func() (types.MessageID, error) {
				return types.MessageID(fmt.Sprintf("msg-compliance-approval-%d", runID)), nil
			},
			func() (types.EventID, error) {
				return types.EventID(fmt.Sprintf("event-compliance-approval-%d", runID)), nil
			},
		),
	)
	tenantID := types.TenantID(fmt.Sprintf("tenant-it-compliance-approval-%d", runID))
	appendInput := testAppendInput(tenantID, "client-compliance-approval", []byte(`{"text":"approve me"}`))
	appendResult, err := repo.AppendMessage(ctx, appendInput)
	if err != nil {
		t.Fatalf("append source message: %v", err)
	}
	approvalID := fmt.Sprintf("approval-set-cancel-%d", runID)
	if _, err := repo.RegisterComplianceExternalProof(ctx, MessageComplianceExternalProofMutationOptions{
		TenantID:         string(tenantID),
		ExternalProofRef: "proof://case/set-cancel",
		Provider:         "legal-proof-system",
		ProofHash:        fmt.Sprintf("sha256:set-cancel-%d", runID),
		OperatorID:       "legal-ops",
		Now:              now,
	}); err != nil {
		t.Fatalf("register compliance proof: %v", err)
	}
	approved, err := repo.ApproveComplianceDelete(ctx, MessageComplianceDeleteApprovalMutationOptions{
		TenantID:         string(tenantID),
		ConversationID:   string(appendInput.Command.ConversationID),
		MessageID:        string(appendResult.MessageID),
		ApprovalID:       approvalID,
		ExternalProofRef: "proof://case/set-cancel",
		OperatorID:       "legal-approver",
		Reason:           "approval reason",
		Now:              now,
	})
	if err != nil {
		t.Fatalf("approve compliance delete: %v", err)
	}
	if approved.Status != MessageComplianceApprovalStatusApproved ||
		approved.ExternalProofRef != "proof://case/set-cancel" ||
		!approved.ReasonPresent {
		t.Fatalf("unexpected approval result: %+v", approved)
	}
	approvedRows, err := repo.AuditComplianceDeleteApprovals(ctx, MessageComplianceDeleteApprovalAuditOptions{
		TenantID:      string(tenantID),
		Status:        MessageComplianceApprovalStatusApproved,
		UpdatedAfter:  timePtr(now.Add(-time.Minute)),
		UpdatedBefore: timePtr(now.Add(time.Minute)),
	})
	if err != nil {
		t.Fatalf("audit approved compliance delete: %v", err)
	}
	if len(approvedRows) != 1 || approvedRows[0].ApprovalID != approvalID {
		t.Fatalf("unexpected approved rows: %+v", approvedRows)
	}
	emptyApprovedRows, err := repo.AuditComplianceDeleteApprovals(ctx, MessageComplianceDeleteApprovalAuditOptions{
		TenantID:     string(tenantID),
		Status:       MessageComplianceApprovalStatusApproved,
		UpdatedAfter: timePtr(now.Add(time.Hour)),
	})
	if err != nil {
		t.Fatalf("audit approved compliance delete outside window: %v", err)
	}
	if len(emptyApprovedRows) != 0 {
		t.Fatalf("expected no approved rows outside updated_at window, got %+v", emptyApprovedRows)
	}
	if _, err := repo.AuditComplianceDeleteApprovals(ctx, MessageComplianceDeleteApprovalAuditOptions{
		TenantID:      string(tenantID),
		UpdatedAfter:  timePtr(now),
		UpdatedBefore: timePtr(now),
	}); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid approval audit time window, got %v", err)
	}

	canceled, err := repo.CancelComplianceDeleteApproval(ctx, MessageComplianceDeleteApprovalMutationOptions{
		TenantID:   string(tenantID),
		ApprovalID: approvalID,
		OperatorID: "legal-approver",
		Now:        now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("cancel compliance delete approval: %v", err)
	}
	if canceled.Status != MessageComplianceApprovalStatusCanceled ||
		canceled.CanceledAt == nil ||
		canceled.CanceledBy != "legal-approver" {
		t.Fatalf("unexpected canceled approval: %+v", canceled)
	}
	_, err = repo.ApproveComplianceDelete(ctx, MessageComplianceDeleteApprovalMutationOptions{
		TenantID:         string(tenantID),
		ConversationID:   string(appendInput.Command.ConversationID),
		MessageID:        string(appendResult.MessageID),
		ApprovalID:       approvalID,
		ExternalProofRef: "proof://case/set-cancel",
		OperatorID:       "legal-approver",
		Reason:           "attempt to resurrect canceled approval",
		Now:              now.Add(2 * time.Minute),
	})
	if !errors.Is(err, types.ErrInvalidMessageState) {
		t.Fatalf("expected canceled approval re-approve to fail, got %v", err)
	}
	canceledRows, err := repo.AuditComplianceDeleteApprovals(ctx, MessageComplianceDeleteApprovalAuditOptions{
		TenantID:   string(tenantID),
		ApprovalID: approvalID,
	})
	if err != nil {
		t.Fatalf("audit canceled compliance delete: %v", err)
	}
	if len(canceledRows) != 1 || canceledRows[0].Status != MessageComplianceApprovalStatusCanceled {
		t.Fatalf("canceled compliance approval should remain canceled, got %+v", canceledRows)
	}
}

func TestMessageRepositoryComplianceExternalProofRevokeBlocksDeleteIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)

	now := time.Date(2026, 6, 17, 4, 0, 0, 0, time.UTC)
	runID := time.Now().UnixNano()
	messageCounter := 0
	eventCounter := 0
	repo := NewMessageRepository(
		pool,
		WithClock(func() time.Time { return now }),
		WithIDGenerators(
			func() (types.MessageID, error) {
				messageCounter++
				return types.MessageID(fmt.Sprintf("msg-compliance-proof-%d-%d", runID, messageCounter)), nil
			},
			func() (types.EventID, error) {
				eventCounter++
				return types.EventID(fmt.Sprintf("event-compliance-proof-%d-%d", runID, eventCounter)), nil
			},
		),
	)
	tenantID := types.TenantID(fmt.Sprintf("tenant-it-compliance-proof-%d", runID))
	appendInput := testAppendInput(tenantID, "client-compliance-proof", []byte(`{"text":"proof gated"}`))
	appendResult, err := repo.AppendMessage(ctx, appendInput)
	if err != nil {
		t.Fatalf("append source message: %v", err)
	}
	proofRef := fmt.Sprintf("proof://case/revoke-%d", runID)
	proof, err := repo.RegisterComplianceExternalProof(ctx, MessageComplianceExternalProofMutationOptions{
		TenantID:         string(tenantID),
		ExternalProofRef: proofRef,
		Provider:         "legal-proof-system",
		ProofHash:        fmt.Sprintf("sha256:revoke-%d", runID),
		OperatorID:       "legal-ops",
		Now:              now,
	})
	if err != nil {
		t.Fatalf("register compliance proof: %v", err)
	}
	if proof.Status != MessageComplianceExternalProofStatusVerified || proof.ProofHash == "" {
		t.Fatalf("unexpected proof registration: %+v", proof)
	}
	approvalID := fmt.Sprintf("approval-proof-revoked-%d", runID)
	if _, err := repo.ApproveComplianceDelete(ctx, MessageComplianceDeleteApprovalMutationOptions{
		TenantID:         string(tenantID),
		ConversationID:   string(appendInput.Command.ConversationID),
		MessageID:        string(appendResult.MessageID),
		ApprovalID:       approvalID,
		ExternalProofRef: proofRef,
		OperatorID:       "legal-approver",
		Reason:           "approved before proof revoke",
		Now:              now,
	}); err != nil {
		t.Fatalf("approve compliance delete: %v", err)
	}
	revoked, err := repo.RevokeComplianceExternalProof(ctx, MessageComplianceExternalProofMutationOptions{
		TenantID:         string(tenantID),
		ExternalProofRef: proofRef,
		OperatorID:       "legal-ops",
		Now:              now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("revoke compliance proof: %v", err)
	}
	if revoked.Status != MessageComplianceExternalProofStatusRevoked || revoked.RevokedAt == nil {
		t.Fatalf("unexpected revoked proof: %+v", revoked)
	}

	deleteInput := testDeleteInput(appendInput, appendResult.MessageID, "delete-revoked-proof-key-1", types.DeleteScopeCompliance, "legal retention cleanup")
	deleteInput.Command.AuthContext.UserID = "compliance-admin"
	deleteInput.Command.ComplianceApprovalID = approvalID
	deleteInput.Command.ExternalProofRef = proofRef
	deleteInput.Permission.OwnershipOverride = true
	deleteInput.Permission.Classification = "COMPLIANCE_RETENTION"
	_, err = repo.DeleteMessage(ctx, deleteInput)
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied for revoked proof, got %v", err)
	}
	assertCurrentSeq(t, ctx, pool, tenantID, appendInput.Command.ConversationID, 1)
	assertCount(t, ctx, pool, "message_change_history", tenantID, 0)

	proofRows, err := repo.AuditComplianceExternalProofs(ctx, MessageComplianceExternalProofAuditOptions{
		TenantID:      string(tenantID),
		Status:        MessageComplianceExternalProofStatusRevoked,
		UpdatedAfter:  timePtr(now.Add(30 * time.Second)),
		UpdatedBefore: timePtr(now.Add(2 * time.Minute)),
	})
	if err != nil {
		t.Fatalf("audit compliance proof: %v", err)
	}
	if len(proofRows) != 1 || proofRows[0].ExternalProofRef != proofRef {
		t.Fatalf("unexpected proof audit rows: %+v", proofRows)
	}
	emptyProofRows, err := repo.AuditComplianceExternalProofs(ctx, MessageComplianceExternalProofAuditOptions{
		TenantID:      string(tenantID),
		Status:        MessageComplianceExternalProofStatusRevoked,
		UpdatedBefore: timePtr(now.Add(30 * time.Second)),
	})
	if err != nil {
		t.Fatalf("audit compliance proof outside window: %v", err)
	}
	if len(emptyProofRows) != 0 {
		t.Fatalf("expected no proof rows outside updated_at window, got %+v", emptyProofRows)
	}
	if _, err := repo.AuditComplianceExternalProofs(ctx, MessageComplianceExternalProofAuditOptions{
		TenantID:      string(tenantID),
		UpdatedAfter:  timePtr(now),
		UpdatedBefore: timePtr(now),
	}); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid proof audit time window, got %v", err)
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
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
