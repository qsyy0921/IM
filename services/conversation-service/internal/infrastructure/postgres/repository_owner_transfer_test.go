package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestRepositoryTransferConversationOwnerIntegration(t *testing.T) {
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	defer pool.Close()

	resetMemberChangeTables(t, ctx, pool)
	ensureOwnerTransferConstraint(t, ctx, pool)
	_, err = pool.Exec(ctx, `
INSERT INTO conversations (
    tenant_id, conversation_id, conversation_type, status, conversation_mode,
    fanout_mode, fanout_policy_version, member_version, permission_version, current_seq_shard
) VALUES ('tenant-owner-transfer', 'conv-owner-transfer', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 5, 7, 'local');
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, join_seq, member_version, permission_version
) VALUES
    ('tenant-owner-transfer', 'conv-owner-transfer', 'owner-1', 'OWNER', 'ACTIVE', 1, 5, 7),
    ('tenant-owner-transfer', 'conv-owner-transfer', 'user-2', 'MEMBER', 'ACTIVE', 2, 5, 7);
`)
	if err != nil {
		t.Fatalf("seed owner transfer conversation: %v", err)
	}

	repository := NewRepository(
		pool,
		WithClock(func() time.Time {
			return time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)
		}),
		WithIDGenerators(
			func() (types.ChangeID, error) { return "change-owner-transfer-1", nil },
			func() (types.EventID, error) { return "event-owner-transfer-1", nil },
		),
	)
	command := types.TransferConversationOwnerCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-owner-transfer",
			UserID:    "owner-1",
			TraceID:   "trace-owner-transfer",
			RequestID: "request-owner-transfer",
		},
		ConversationID:        "conv-owner-transfer",
		NewOwnerUserID:        "user-2",
		ExpectedMemberVersion: 5,
		IdempotencyKey:        "idem-owner-transfer-1",
		Reason:                "planned handoff",
	}

	result, err := repository.TransferConversationOwner(ctx, command)
	if err != nil {
		t.Fatalf("transfer conversation owner: %v", err)
	}
	if result.ChangeID != "change-owner-transfer-1" ||
		result.PreviousOwnerUserID != "owner-1" ||
		result.NewOwnerUserID != "user-2" ||
		result.BoundarySeq != 1 ||
		result.MemberVersion != 6 ||
		result.PermissionVersion != 8 ||
		result.Status != types.MemberChangeStatusOutboxEnqueued ||
		result.IdempotentReplay {
		t.Fatalf("unexpected transfer result: %+v", result)
	}

	replay, err := repository.TransferConversationOwner(ctx, command)
	if err != nil {
		t.Fatalf("replay owner transfer: %v", err)
	}
	if !replay.IdempotentReplay ||
		replay.ChangeID != result.ChangeID ||
		replay.BoundarySeq != result.BoundarySeq ||
		replay.MemberVersion != result.MemberVersion ||
		replay.PermissionVersion != result.PermissionVersion {
		t.Fatalf("unexpected replay result: %+v", replay)
	}

	rows, err := pool.Query(ctx, `
SELECT user_id, role, status, member_version, permission_version
FROM conversation_members
WHERE tenant_id = 'tenant-owner-transfer'
  AND conversation_id = 'conv-owner-transfer'
ORDER BY user_id
`)
	if err != nil {
		t.Fatalf("query transfer members: %v", err)
	}
	defer rows.Close()
	members := map[string]struct {
		role              types.MemberRole
		status            types.MemberStatus
		memberVersion     int64
		permissionVersion int64
	}{}
	for rows.Next() {
		var userID string
		value := struct {
			role              types.MemberRole
			status            types.MemberStatus
			memberVersion     int64
			permissionVersion int64
		}{}
		if err := rows.Scan(&userID, &value.role, &value.status, &value.memberVersion, &value.permissionVersion); err != nil {
			t.Fatalf("scan transfer member: %v", err)
		}
		members[userID] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("transfer member rows: %v", err)
	}
	if members["owner-1"].role != types.MemberRoleAdmin ||
		members["owner-1"].status != types.MemberStatusActive ||
		members["owner-1"].memberVersion != 6 ||
		members["user-2"].role != types.MemberRoleOwner ||
		members["user-2"].status != types.MemberStatusActive ||
		members["user-2"].memberVersion != 6 {
		t.Fatalf("unexpected transfer members: %+v", members)
	}

	var currentSeq, conversationMemberVersion, conversationPermissionVersion int64
	if err := pool.QueryRow(ctx, `
SELECT cs.current_seq, c.member_version, c.permission_version
FROM conversation_seq cs
JOIN conversations c
  ON c.tenant_id = cs.tenant_id
 AND c.conversation_id = cs.conversation_id
WHERE cs.tenant_id = 'tenant-owner-transfer'
  AND cs.conversation_id = 'conv-owner-transfer'
`).Scan(&currentSeq, &conversationMemberVersion, &conversationPermissionVersion); err != nil {
		t.Fatalf("query transfer versions: %v", err)
	}
	if currentSeq != 1 || conversationMemberVersion != 6 || conversationPermissionVersion != 8 {
		t.Fatalf("unexpected transfer versions: seq=%d member=%d permission=%d", currentSeq, conversationMemberVersion, conversationPermissionVersion)
	}

	var sagaStatus types.MemberChangeStatus
	var sagaChangeType types.MemberChangeType
	var previousOwnerNewRole, newOwnerNewRole string
	if err := pool.QueryRow(ctx, `
SELECT
    status,
    change_type,
    metadata_json->>'previous_owner_new_role',
    metadata_json->>'new_owner_new_role'
FROM member_change_saga
WHERE tenant_id = 'tenant-owner-transfer'
  AND conversation_id = 'conv-owner-transfer'
  AND idempotency_key = 'idem-owner-transfer-1'
`).Scan(&sagaStatus, &sagaChangeType, &previousOwnerNewRole, &newOwnerNewRole); err != nil {
		t.Fatalf("query transfer saga: %v", err)
	}
	if sagaStatus != types.MemberChangeStatusOutboxEnqueued ||
		sagaChangeType != types.MemberChangeTypeOwnerTransfer ||
		previousOwnerNewRole != "ADMIN" ||
		newOwnerNewRole != "OWNER" {
		t.Fatalf("unexpected transfer saga: status=%s type=%s previous=%s new=%s", sagaStatus, sagaChangeType, previousOwnerNewRole, newOwnerNewRole)
	}

	var timelineType, outboxType types.TimelineEventType
	var previousOwnerUserID, newOwnerUserID string
	if err := pool.QueryRow(ctx, `
SELECT event_type, payload_json->>'previous_owner_user_id', payload_json->>'new_owner_user_id'
FROM conversation_timeline_events
WHERE tenant_id = 'tenant-owner-transfer'
  AND conversation_id = 'conv-owner-transfer'
  AND event_id = 'event-owner-transfer-1'
`).Scan(&timelineType, &previousOwnerUserID, &newOwnerUserID); err != nil {
		t.Fatalf("query transfer timeline: %v", err)
	}
	if timelineType != types.TimelineEventConversationMemberOwnerTransferred ||
		previousOwnerUserID != "owner-1" ||
		newOwnerUserID != "user-2" {
		t.Fatalf("unexpected transfer timeline: type=%s previous=%s new=%s", timelineType, previousOwnerUserID, newOwnerUserID)
	}
	if err := pool.QueryRow(ctx, `
SELECT event_type
FROM message_outbox
WHERE tenant_id = 'tenant-owner-transfer'
  AND event_id = 'event-owner-transfer-1'
`).Scan(&outboxType); err != nil {
		t.Fatalf("query transfer outbox: %v", err)
	}
	if outboxType != types.TimelineEventConversationMemberOwnerTransferred {
		t.Fatalf("unexpected transfer outbox type: %s", outboxType)
	}

	conflict := command
	conflict.NewOwnerUserID = "user-3"
	_, err = repository.TransferConversationOwner(ctx, conflict)
	if !errors.Is(err, types.ErrMemberConflict) {
		t.Fatalf("expected owner transfer idempotency conflict, got %v", err)
	}

	stats, err := repository.MarkPublishedMemberChanges(ctx, 100)
	if err != nil {
		t.Fatalf("mark transfer before publish: %v", err)
	}
	if stats.Advanced != 0 {
		t.Fatalf("expected no advance before publish, got %+v", stats)
	}
	if _, err := pool.Exec(ctx, `
UPDATE message_outbox
SET status = 'PUBLISHED',
    published_at = now()
WHERE tenant_id = 'tenant-owner-transfer'
  AND event_id = 'event-owner-transfer-1'
`); err != nil {
		t.Fatalf("publish transfer outbox: %v", err)
	}
	stats, err = repository.MarkPublishedMemberChanges(ctx, 100)
	if err != nil {
		t.Fatalf("mark transfer after publish: %v", err)
	}
	if stats.Advanced != 1 {
		t.Fatalf("expected one transfer advance, got %+v", stats)
	}
	assertMemberChangeStatus(t, ctx, pool, result.ChangeID, types.MemberChangeStatusDone, true)
}

func TestRepositoryTransferConversationOwnerRejectsInvalidTargetsWithoutSideEffectsIntegration(t *testing.T) {
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	defer pool.Close()

	cases := []struct {
		name          string
		targetRole    types.MemberRole
		targetStatus  types.MemberStatus
		wantErr       error
		idempotencyID string
	}{
		{
			name:          "inactive target",
			targetRole:    types.MemberRoleMember,
			targetStatus:  types.MemberStatusLeft,
			wantErr:       types.ErrMemberConflict,
			idempotencyID: "idem-owner-transfer-inactive",
		},
		{
			name:          "target already owner",
			targetRole:    types.MemberRoleOwner,
			targetStatus:  types.MemberStatusActive,
			wantErr:       types.ErrPermissionDenied,
			idempotencyID: "idem-owner-transfer-existing-owner",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetMemberChangeTables(t, ctx, pool)
			ensureOwnerTransferConstraint(t, ctx, pool)
			if _, err := pool.Exec(ctx, `
INSERT INTO conversations (
    tenant_id, conversation_id, conversation_type, status, conversation_mode,
    fanout_mode, fanout_policy_version, member_version, permission_version, current_seq_shard
) VALUES ('tenant-owner-transfer-negative', 'conv-owner-transfer-negative', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 5, 7, 'local');
`); err != nil {
				t.Fatalf("seed invalid owner transfer conversation: %v", err)
			}
			if _, err := pool.Exec(ctx, `
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, join_seq, member_version, permission_version
) VALUES
    ('tenant-owner-transfer-negative', 'conv-owner-transfer-negative', 'owner-1', 'OWNER', 'ACTIVE', 1, 5, 7),
    ('tenant-owner-transfer-negative', 'conv-owner-transfer-negative', 'user-2', $1, $2, 2, 5, 7);
`, tc.targetRole, tc.targetStatus); err != nil {
				t.Fatalf("seed invalid owner transfer members: %v", err)
			}

			repository := NewRepository(
				pool,
				WithClock(func() time.Time {
					return time.Date(2026, 6, 10, 11, 0, 0, 0, time.UTC)
				}),
				WithIDGenerators(
					func() (types.ChangeID, error) { return "change-owner-transfer-negative", nil },
					func() (types.EventID, error) { return "event-owner-transfer-negative", nil },
				),
			)
			_, err := repository.TransferConversationOwner(ctx, types.TransferConversationOwnerCommand{
				AuthContext: types.AuthContext{
					TenantID: "tenant-owner-transfer-negative",
					UserID:   "owner-1",
				},
				ConversationID:        "conv-owner-transfer-negative",
				NewOwnerUserID:        "user-2",
				ExpectedMemberVersion: 5,
				IdempotencyKey:        tc.idempotencyID,
				Reason:                "invalid target",
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			assertNoOwnerTransferSideEffects(t, ctx, pool)
		})
	}
}

func ensureOwnerTransferConstraint(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
ALTER TABLE member_change_saga
    DROP CONSTRAINT IF EXISTS member_change_saga_change_type_check;

ALTER TABLE member_change_saga
    ADD CONSTRAINT member_change_saga_change_type_check CHECK (
        change_type IN ('JOIN', 'LEAVE', 'ROLE_CHANGED', 'REMOVE', 'OWNER_TRANSFER')
    );
`); err != nil {
		t.Fatalf("ensure owner transfer constraint: %v", err)
	}
}

func assertNoOwnerTransferSideEffects(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for tableName, query := range map[string]string{
		"conversation_seq": `
SELECT count(*)
FROM conversation_seq
WHERE tenant_id = 'tenant-owner-transfer-negative'
  AND conversation_id = 'conv-owner-transfer-negative'
`,
		"member_change_saga": `
SELECT count(*)
FROM member_change_saga
WHERE tenant_id = 'tenant-owner-transfer-negative'
  AND conversation_id = 'conv-owner-transfer-negative'
`,
		"conversation_timeline_events": `
SELECT count(*)
FROM conversation_timeline_events
WHERE tenant_id = 'tenant-owner-transfer-negative'
  AND conversation_id = 'conv-owner-transfer-negative'
`,
		"message_outbox": `
SELECT count(*)
FROM message_outbox
WHERE tenant_id = 'tenant-owner-transfer-negative'
  AND conversation_id = 'conv-owner-transfer-negative'
`,
	} {
		var count int
		if err := pool.QueryRow(ctx, query).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", tableName, err)
		}
		if count != 0 {
			t.Fatalf("expected no %s side effects, got %d rows", tableName, count)
		}
	}

	rows, err := pool.Query(ctx, `
SELECT user_id, role, status, member_version, permission_version
FROM conversation_members
WHERE tenant_id = 'tenant-owner-transfer-negative'
  AND conversation_id = 'conv-owner-transfer-negative'
ORDER BY user_id
`)
	if err != nil {
		t.Fatalf("query member side effects: %v", err)
	}
	defer rows.Close()
	members := map[string]struct {
		role              types.MemberRole
		status            types.MemberStatus
		memberVersion     int64
		permissionVersion int64
	}{}
	for rows.Next() {
		var userID string
		value := struct {
			role              types.MemberRole
			status            types.MemberStatus
			memberVersion     int64
			permissionVersion int64
		}{}
		if err := rows.Scan(&userID, &value.role, &value.status, &value.memberVersion, &value.permissionVersion); err != nil {
			t.Fatalf("scan member side effects: %v", err)
		}
		members[userID] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("member side effect rows: %v", err)
	}
	if members["owner-1"].role != types.MemberRoleOwner ||
		members["owner-1"].status != types.MemberStatusActive ||
		members["owner-1"].memberVersion != 5 ||
		members["owner-1"].permissionVersion != 7 ||
		members["user-2"].memberVersion != 5 ||
		members["user-2"].permissionVersion != 7 {
		t.Fatalf("unexpected member mutations after rejected transfer: %+v", members)
	}
}
