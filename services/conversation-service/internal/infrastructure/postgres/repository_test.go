package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestRepositoryGetSendContextIntegration(t *testing.T) {
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

	resetConversationTables(t, ctx, pool)
	_, err = pool.Exec(ctx, `
INSERT INTO conversations (
    tenant_id, conversation_id, conversation_type, status, conversation_mode,
    fanout_mode, fanout_policy_version, member_version, permission_version, current_seq_shard
) VALUES ('tenant-1', 'conv-1', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 5, 7, 'local');
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, member_version, permission_version
) VALUES ('tenant-1', 'conv-1', 'user-1', 'MEMBER', 'ACTIVE', 5, 7);
`)
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	result, err := NewRepository(pool).GetSendContext(ctx, types.GetSendContextCommand{
		TenantID:       "tenant-1",
		ConversationID: "conv-1",
		UserID:         "user-1",
	})
	if err != nil {
		t.Fatalf("get send context: %v", err)
	}
	if result.MemberVersion != 5 ||
		result.PermissionVersion != 7 ||
		result.ConversationMode != types.ConversationModeLocalRowLock ||
		result.FanoutMode != types.FanoutModeWriteFanout ||
		result.FanoutPolicyVersion != 3 ||
		result.CurrentSeqShard != "local" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRepositoryGetSendContextErrorIntegration(t *testing.T) {
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

	resetConversationTables(t, ctx, pool)
	_, err = pool.Exec(ctx, `
INSERT INTO conversations (
    tenant_id, conversation_id, conversation_type, status, conversation_mode,
    fanout_mode, fanout_policy_version, member_version, permission_version, current_seq_shard
) VALUES
    ('tenant-1', 'archived-conv', 'GROUP', 'ARCHIVED', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 5, 7, 'local'),
    ('tenant-1', 'active-conv', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 5, 7, 'local');
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, member_version, permission_version
) VALUES
    ('tenant-1', 'active-conv', 'user-left', 'MEMBER', 'LEFT', 5, 7);
`)
	if err != nil {
		t.Fatalf("seed conversation errors: %v", err)
	}

	repository := NewRepository(pool)
	cases := []struct {
		name    string
		command types.GetSendContextCommand
		wantErr error
	}{
		{
			name: "conversation missing",
			command: types.GetSendContextCommand{
				TenantID:       "tenant-1",
				ConversationID: "missing-conv",
				UserID:         "user-1",
			},
			wantErr: types.ErrConversationNotFound,
		},
		{
			name: "conversation archived",
			command: types.GetSendContextCommand{
				TenantID:       "tenant-1",
				ConversationID: "archived-conv",
				UserID:         "user-1",
			},
			wantErr: types.ErrConversationNotFound,
		},
		{
			name: "member left",
			command: types.GetSendContextCommand{
				TenantID:       "tenant-1",
				ConversationID: "active-conv",
				UserID:         "user-left",
			},
			wantErr: types.ErrMemberNotActive,
		},
		{
			name: "member missing",
			command: types.GetSendContextCommand{
				TenantID:       "tenant-1",
				ConversationID: "active-conv",
				UserID:         "missing-user",
			},
			wantErr: types.ErrMemberNotActive,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repository.GetSendContext(ctx, tc.command)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestRepositoryCreateMemberChangeIntegration(t *testing.T) {
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
	_, err = pool.Exec(ctx, `
INSERT INTO conversations (
    tenant_id, conversation_id, conversation_type, status, conversation_mode,
    fanout_mode, fanout_policy_version, member_version, permission_version, current_seq_shard
) VALUES ('tenant-member', 'conv-member', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 5, 7, 'local');
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, member_version, permission_version
) VALUES ('tenant-member', 'conv-member', 'owner-1', 'OWNER', 'ACTIVE', 5, 7);
`)
	if err != nil {
		t.Fatalf("seed member change conversation: %v", err)
	}

	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	repository := NewRepository(
		pool,
		WithClock(func() time.Time { return now }),
		WithIDGenerators(
			func() (types.ChangeID, error) { return "change-1", nil },
			func() (types.EventID, error) { return "event-1", nil },
		),
	)
	command := types.CreateMemberChangeCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-member",
			UserID:    "owner-1",
			TraceID:   "trace-1",
			RequestID: "request-1",
		},
		ConversationID:        "conv-member",
		TargetUserID:          "target-1",
		ChangeType:            types.MemberChangeTypeJoin,
		TargetRole:            types.MemberRoleMember,
		ExpectedMemberVersion: 5,
		IdempotencyKey:        "idem-join-1",
		ConflictPolicy:        types.MemberChangeConflictPolicyReject,
		Reason:                "invite target",
	}

	result, err := repository.CreateMemberChange(ctx, command)
	if err != nil {
		t.Fatalf("create member change: %v", err)
	}
	if result.ChangeID != "change-1" ||
		result.BoundarySeq != 1 ||
		result.MemberVersion != 6 ||
		result.PermissionVersion != 8 ||
		result.Status != types.MemberChangeStatusOutboxEnqueued ||
		result.IdempotentReplay {
		t.Fatalf("unexpected result: %+v", result)
	}

	replay, err := repository.CreateMemberChange(ctx, command)
	if err != nil {
		t.Fatalf("replay member change: %v", err)
	}
	if !replay.IdempotentReplay ||
		replay.ChangeID != result.ChangeID ||
		replay.BoundarySeq != result.BoundarySeq ||
		replay.MemberVersion != result.MemberVersion ||
		replay.PermissionVersion != result.PermissionVersion {
		t.Fatalf("unexpected replay result: %+v", replay)
	}

	conflict := command
	conflict.TargetRole = types.MemberRoleAdmin
	_, err = repository.CreateMemberChange(ctx, conflict)
	if !errors.Is(err, types.ErrMemberConflict) {
		t.Fatalf("expected member conflict for same idempotency key, got %v", err)
	}

	var currentSeq int64
	if err := pool.QueryRow(ctx, `
SELECT current_seq
FROM conversation_seq
WHERE tenant_id = 'tenant-member'
  AND conversation_id = 'conv-member'
`).Scan(&currentSeq); err != nil {
		t.Fatalf("query conversation seq: %v", err)
	}
	if currentSeq != 1 {
		t.Fatalf("expected current_seq=1, got %d", currentSeq)
	}
	assertMemberChangeCounts(t, ctx, pool)

	var conversationMemberVersion, conversationPermissionVersion int64
	if err := pool.QueryRow(ctx, `
SELECT member_version, permission_version
FROM conversations
WHERE tenant_id = 'tenant-member'
  AND conversation_id = 'conv-member'
`).Scan(&conversationMemberVersion, &conversationPermissionVersion); err != nil {
		t.Fatalf("query conversation versions: %v", err)
	}
	if conversationMemberVersion != 6 || conversationPermissionVersion != 8 {
		t.Fatalf("unexpected conversation versions: member=%d permission=%d", conversationMemberVersion, conversationPermissionVersion)
	}

	var memberRole types.MemberRole
	var memberStatus types.MemberStatus
	var joinSeq sql.NullInt64
	var memberVersion, permissionVersion int64
	if err := pool.QueryRow(ctx, `
SELECT role, status, join_seq, member_version, permission_version
FROM conversation_members
WHERE tenant_id = 'tenant-member'
  AND conversation_id = 'conv-member'
  AND user_id = 'target-1'
`).Scan(&memberRole, &memberStatus, &joinSeq, &memberVersion, &permissionVersion); err != nil {
		t.Fatalf("query target member: %v", err)
	}
	if memberRole != types.MemberRoleMember ||
		memberStatus != types.MemberStatusActive ||
		!joinSeq.Valid ||
		joinSeq.Int64 != 1 ||
		memberVersion != 6 ||
		permissionVersion != 8 {
		t.Fatalf("unexpected target member: role=%s status=%s join=%v member=%d permission=%d", memberRole, memberStatus, joinSeq, memberVersion, permissionVersion)
	}

	var sagaStatus types.MemberChangeStatus
	var sagaBoundarySeq int64
	var timelineEventID, outboxEventID string
	var metadataMemberVersion, metadataPermissionVersion int64
	if err := pool.QueryRow(ctx, `
SELECT
    status,
    boundary_seq,
    timeline_event_id,
    outbox_event_id,
    (metadata_json->>'member_version')::bigint,
    (metadata_json->>'permission_version')::bigint
FROM member_change_saga
WHERE tenant_id = 'tenant-member'
  AND conversation_id = 'conv-member'
  AND idempotency_key = 'idem-join-1'
`).Scan(
		&sagaStatus,
		&sagaBoundarySeq,
		&timelineEventID,
		&outboxEventID,
		&metadataMemberVersion,
		&metadataPermissionVersion,
	); err != nil {
		t.Fatalf("query saga: %v", err)
	}
	if sagaStatus != types.MemberChangeStatusOutboxEnqueued ||
		sagaBoundarySeq != 1 ||
		timelineEventID != "event-1" ||
		outboxEventID != "event-1" ||
		metadataMemberVersion != 6 ||
		metadataPermissionVersion != 8 {
		t.Fatalf("unexpected saga: status=%s seq=%d timeline=%s outbox=%s member=%d permission=%d", sagaStatus, sagaBoundarySeq, timelineEventID, outboxEventID, metadataMemberVersion, metadataPermissionVersion)
	}

	var timelineSeq int64
	var timelineEventType types.TimelineEventType
	var timelineMessageID sql.NullString
	var timelineActorID string
	var timelinePermissionVersion int64
	var timelineChangeID string
	if err := pool.QueryRow(ctx, `
SELECT
    seq,
    event_type,
    message_id,
    actor_id,
    permission_version,
    payload_json->>'change_id'
FROM conversation_timeline_events
WHERE tenant_id = 'tenant-member'
  AND conversation_id = 'conv-member'
  AND event_id = 'event-1'
`).Scan(
		&timelineSeq,
		&timelineEventType,
		&timelineMessageID,
		&timelineActorID,
		&timelinePermissionVersion,
		&timelineChangeID,
	); err != nil {
		t.Fatalf("query timeline event: %v", err)
	}
	if timelineSeq != 1 ||
		timelineEventType != types.TimelineEventConversationMemberJoined ||
		timelineMessageID.Valid ||
		timelineActorID != "owner-1" ||
		timelinePermissionVersion != 8 ||
		timelineChangeID != "change-1" {
		t.Fatalf("unexpected timeline event: seq=%d type=%s message=%v actor=%s permission=%d change=%s", timelineSeq, timelineEventType, timelineMessageID, timelineActorID, timelinePermissionVersion, timelineChangeID)
	}

	var outboxAggregateVersion int64
	var outboxEventType types.TimelineEventType
	var outboxPartitionKey, outboxProducer, outboxStatus, outboxChangeID string
	if err := pool.QueryRow(ctx, `
SELECT
    aggregate_version,
    event_type,
    partition_key,
    producer,
    status,
    payload_json->>'change_id'
FROM message_outbox
WHERE event_id = 'event-1'
`).Scan(
		&outboxAggregateVersion,
		&outboxEventType,
		&outboxPartitionKey,
		&outboxProducer,
		&outboxStatus,
		&outboxChangeID,
	); err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	if outboxAggregateVersion != 1 ||
		outboxEventType != types.TimelineEventConversationMemberJoined ||
		outboxPartitionKey != "tenant-member:conv-member" ||
		outboxProducer != "conversation-service" ||
		outboxStatus != "PENDING" ||
		outboxChangeID != "change-1" {
		t.Fatalf("unexpected outbox: version=%d type=%s key=%s producer=%s status=%s change=%s", outboxAggregateVersion, outboxEventType, outboxPartitionKey, outboxProducer, outboxStatus, outboxChangeID)
	}
}

func resetConversationTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
TRUNCATE TABLE
    member_change_saga,
    conversation_members,
    conversations
CASCADE
`); err != nil {
		t.Fatalf("truncate conversation tables: %v", err)
	}
}

func resetMemberChangeTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
TRUNCATE TABLE
    message_outbox,
    conversation_timeline_events,
    conversation_seq,
    member_change_saga,
    conversation_members,
    conversations
CASCADE
`); err != nil {
		t.Fatalf("truncate member change tables: %v", err)
	}
}

func assertMemberChangeCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for tableName, want := range map[string]int{
		"member_change_saga":           1,
		"conversation_timeline_events": 1,
		"message_outbox":               1,
	} {
		var count int
		query := "SELECT COUNT(*) FROM " + tableName + " WHERE tenant_id = 'tenant-member'"
		if err := pool.QueryRow(ctx, query).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", tableName, err)
		}
		if count != want {
			t.Fatalf("expected %s count %d, got %d", tableName, want, count)
		}
	}
}
