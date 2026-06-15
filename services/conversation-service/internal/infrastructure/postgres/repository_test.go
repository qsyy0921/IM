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
		result.CurrentSeqShard != "local" ||
		result.DirectPeerUserID != "" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRepositoryGetSendContextDirectPeerIntegration(t *testing.T) {
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
) VALUES ('tenant-1', 'conv-direct', 'DIRECT', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 5, 7, 'local');
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, member_version, permission_version
) VALUES
    ('tenant-1', 'conv-direct', 'user-1', 'MEMBER', 'ACTIVE', 5, 7),
    ('tenant-1', 'conv-direct', 'user-2', 'MEMBER', 'ACTIVE', 5, 7);
`)
	if err != nil {
		t.Fatalf("seed direct conversation: %v", err)
	}

	result, err := NewRepository(pool).GetSendContext(ctx, types.GetSendContextCommand{
		TenantID:       "tenant-1",
		ConversationID: "conv-direct",
		UserID:         "user-1",
	})
	if err != nil {
		t.Fatalf("get send context: %v", err)
	}
	if result.DirectPeerUserID != "user-2" {
		t.Fatalf("expected direct peer user-2, got %+v", result)
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

func TestRepositoryListConversationMembersIntegration(t *testing.T) {
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
    ('tenant-list', 'conv-list', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 10, 12, 'local'),
    ('tenant-list', 'conv-archived', 'GROUP', 'ARCHIVED', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 10, 12, 'local');
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, join_seq, leave_seq, member_version, permission_version, updated_at
) VALUES
    ('tenant-list', 'conv-list', 'admin-1', 'ADMIN', 'ACTIVE', 2, NULL, 10, 12, '2026-06-10T12:00:00Z'),
    ('tenant-list', 'conv-list', 'member-1', 'MEMBER', 'ACTIVE', 3, NULL, 10, 12, '2026-06-10T12:01:00Z'),
    ('tenant-list', 'conv-list', 'member-2', 'MEMBER', 'ACTIVE', 4, NULL, 10, 12, '2026-06-10T12:02:00Z'),
    ('tenant-list', 'conv-list', 'owner-1', 'OWNER', 'ACTIVE', 1, NULL, 10, 12, '2026-06-10T11:59:00Z'),
    ('tenant-list', 'conv-list', 'left-1', 'MEMBER', 'LEFT', 1, 5, 9, 11, '2026-06-10T12:03:00Z'),
    ('tenant-list', 'conv-list', 'banned-1', 'MEMBER', 'BANNED', 1, 6, 9, 11, '2026-06-10T12:04:00Z'),
    ('tenant-list', 'conv-archived', 'owner-1', 'OWNER', 'ACTIVE', 1, NULL, 10, 12, '2026-06-10T12:00:00Z');
`)
	if err != nil {
		t.Fatalf("seed members: %v", err)
	}

	repository := NewRepository(pool)
	firstPage, err := repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       2,
	})
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	if firstPage.TenantID != "tenant-list" ||
		firstPage.ConversationID != "conv-list" ||
		firstPage.MemberVersion != 10 ||
		firstPage.PermissionVersion != 12 ||
		firstPage.NextPageToken == "" ||
		len(firstPage.Members) != 2 {
		t.Fatalf("unexpected first page: %+v", firstPage)
	}
	if firstPage.Members[0].UserID != "admin-1" ||
		firstPage.Members[1].UserID != "member-1" {
		t.Fatalf("unexpected first page order: %+v", firstPage.Members)
	}
	for _, member := range firstPage.Members {
		if member.Status != types.MemberStatusActive {
			t.Fatalf("expected only active members, got %+v", member)
		}
	}

	secondPage, err := repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       2,
		PageToken:      firstPage.NextPageToken,
	})
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}
	if secondPage.NextPageToken != "" || len(secondPage.Members) != 2 {
		t.Fatalf("unexpected second page: %+v", secondPage)
	}
	if secondPage.Members[0].UserID != "member-2" ||
		secondPage.Members[1].UserID != "owner-1" {
		t.Fatalf("unexpected second page order: %+v", secondPage.Members)
	}
	if secondPage.Members[1].Role != types.MemberRoleOwner ||
		secondPage.Members[1].JoinSeq != 1 ||
		secondPage.Members[1].MemberVersion != 10 ||
		secondPage.Members[1].PermissionVersion != 12 {
		t.Fatalf("unexpected owner member: %+v", secondPage.Members[1])
	}

	adminOnly, err := repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       10,
		RoleFilter:     types.MemberRoleAdmin,
	})
	if err != nil {
		t.Fatalf("list admin members: %v", err)
	}
	if len(adminOnly.Members) != 1 || adminOnly.Members[0].UserID != "admin-1" || adminOnly.Members[0].Role != types.MemberRoleAdmin {
		t.Fatalf("unexpected admin members: %+v", adminOnly.Members)
	}

	memberFirstPage, err := repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       1,
		RoleFilter:     types.MemberRoleMember,
	})
	if err != nil {
		t.Fatalf("list first member page: %v", err)
	}
	if len(memberFirstPage.Members) != 1 ||
		memberFirstPage.Members[0].UserID != "member-1" ||
		memberFirstPage.NextPageToken == "" {
		t.Fatalf("unexpected first member page: %+v", memberFirstPage)
	}
	memberSecondPage, err := repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       1,
		PageToken:      memberFirstPage.NextPageToken,
		RoleFilter:     types.MemberRoleMember,
	})
	if err != nil {
		t.Fatalf("list second member page: %v", err)
	}
	if len(memberSecondPage.Members) != 1 ||
		memberSecondPage.Members[0].UserID != "member-2" ||
		memberSecondPage.NextPageToken != "" {
		t.Fatalf("unexpected second member page: %+v", memberSecondPage)
	}
	_, err = repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       1,
		PageToken:      memberFirstPage.NextPageToken,
		RoleFilter:     types.MemberRoleAdmin,
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor when role_filter changes, got %v", err)
	}

	_, err = repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "left-1",
		},
		ConversationID: "conv-list",
	})
	if !errors.Is(err, types.ErrMemberNotActive) {
		t.Fatalf("expected member not active for left caller, got %v", err)
	}

	_, err = repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-archived",
	})
	if !errors.Is(err, types.ErrConversationNotFound) {
		t.Fatalf("expected archived conversation not found, got %v", err)
	}

	_, err = repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageToken:      "not-base64",
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid page token, got %v", err)
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

	detail, err := repository.GetMemberChange(ctx, types.GetMemberChangeCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-member",
			UserID:   "owner-1",
		},
		ConversationID: "conv-member",
		ChangeID:       result.ChangeID,
	})
	if err != nil {
		t.Fatalf("get member change: %v", err)
	}
	if detail.ChangeID != result.ChangeID ||
		detail.TargetUserID != "target-1" ||
		detail.OperatorUserID != "owner-1" ||
		detail.ChangeType != types.MemberChangeTypeJoin ||
		detail.Status != types.MemberChangeStatusOutboxEnqueued ||
		detail.BoundarySeq != 1 ||
		detail.MemberVersion != 6 ||
		detail.PermissionVersion != 8 ||
		detail.OldRole != "" ||
		detail.NewRole != types.MemberRoleMember ||
		detail.Reason != "invite target" {
		t.Fatalf("unexpected member change detail: %+v", detail)
	}

	targetDetail, err := repository.GetMemberChange(ctx, types.GetMemberChangeCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-member",
			UserID:   "target-1",
		},
		ConversationID: "conv-member",
		ChangeID:       result.ChangeID,
	})
	if err != nil {
		t.Fatalf("target get member change: %v", err)
	}
	if targetDetail.ChangeID != result.ChangeID {
		t.Fatalf("unexpected target detail: %+v", targetDetail)
	}

	_, err = pool.Exec(ctx, `
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, member_version, permission_version
) VALUES ('tenant-member', 'conv-member', 'admin-1', 'ADMIN', 'ACTIVE', 6, 8)
`)
	if err != nil {
		t.Fatalf("seed admin member: %v", err)
	}
	adminDetail, err := repository.GetMemberChange(ctx, types.GetMemberChangeCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-member",
			UserID:   "admin-1",
		},
		ConversationID: "conv-member",
		ChangeID:       result.ChangeID,
	})
	if err != nil {
		t.Fatalf("admin get member change: %v", err)
	}
	if adminDetail.ChangeID != result.ChangeID {
		t.Fatalf("unexpected admin detail: %+v", adminDetail)
	}

	_, err = repository.GetMemberChange(ctx, types.GetMemberChangeCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-member",
			UserID:   "stranger-1",
		},
		ConversationID: "conv-member",
		ChangeID:       result.ChangeID,
	})
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied for stranger, got %v", err)
	}

	_, err = pool.Exec(ctx, `
UPDATE member_change_saga
SET last_error = 'duplicate key value violates unique constraint member_change_saga_command_hash_key'
WHERE change_id = $1
`, result.ChangeID)
	if err != nil {
		t.Fatalf("seed raw last error: %v", err)
	}
	detail, err = repository.GetMemberChange(ctx, types.GetMemberChangeCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-member",
			UserID:   "owner-1",
		},
		ConversationID: "conv-member",
		ChangeID:       result.ChangeID,
	})
	if err != nil {
		t.Fatalf("get member change with last error: %v", err)
	}
	if detail.LastError != "member change processing failed" {
		t.Fatalf("expected sanitized last error, got %q", detail.LastError)
	}

	_, err = repository.GetMemberChange(ctx, types.GetMemberChangeCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-member",
			UserID:   "owner-1",
		},
		ConversationID: "conv-member",
		ChangeID:       "missing-change",
	})
	if !errors.Is(err, types.ErrMemberChangeNotFound) {
		t.Fatalf("expected member change not found, got %v", err)
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

func TestRepositoryMarkPublishedMemberChangesIntegration(t *testing.T) {
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
	seedMemberChangeConversation(t, ctx, pool)
	repository := NewRepository(
		pool,
		WithClock(func() time.Time {
			return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
		}),
		WithIDGenerators(
			func() (types.ChangeID, error) { return "change-progress-1", nil },
			func() (types.EventID, error) { return "event-progress-1", nil },
		),
	)
	command := types.CreateMemberChangeCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-member",
			UserID:    "owner-1",
			TraceID:   "trace-progress",
			RequestID: "request-progress",
		},
		ConversationID:        "conv-member",
		TargetUserID:          "target-progress-1",
		ChangeType:            types.MemberChangeTypeJoin,
		TargetRole:            types.MemberRoleMember,
		ExpectedMemberVersion: 5,
		IdempotencyKey:        "idem-progress-1",
		ConflictPolicy:        types.MemberChangeConflictPolicyReject,
		Reason:                "invite progress",
	}
	result, err := repository.CreateMemberChange(ctx, command)
	if err != nil {
		t.Fatalf("create member change: %v", err)
	}

	stats, err := repository.MarkPublishedMemberChanges(ctx, 100)
	if err != nil {
		t.Fatalf("mark before outbox published: %v", err)
	}
	if stats.Advanced != 0 {
		t.Fatalf("expected no advance before outbox published, got %+v", stats)
	}
	assertMemberChangeStatus(t, ctx, pool, result.ChangeID, types.MemberChangeStatusOutboxEnqueued, false)

	if _, err := pool.Exec(ctx, `
UPDATE message_outbox
SET status = 'PUBLISHED',
    published_at = now()
WHERE tenant_id = 'tenant-member'
  AND event_id = 'event-progress-1'
`); err != nil {
		t.Fatalf("publish outbox: %v", err)
	}

	stats, err = repository.MarkPublishedMemberChanges(ctx, 100)
	if err != nil {
		t.Fatalf("mark after outbox published: %v", err)
	}
	if stats.Advanced != 1 {
		t.Fatalf("expected one advance, got %+v", stats)
	}
	assertMemberChangeStatus(t, ctx, pool, result.ChangeID, types.MemberChangeStatusDone, true)

	stats, err = repository.MarkPublishedMemberChanges(ctx, 100)
	if err != nil {
		t.Fatalf("mark idempotent: %v", err)
	}
	if stats.Advanced != 0 {
		t.Fatalf("expected no second advance, got %+v", stats)
	}
}

func TestRepositoryMarkPublishedMemberChangesHonorsLimit(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `
INSERT INTO member_change_saga (
    change_id, tenant_id, conversation_id, user_id, change_type, status,
    idempotency_key, command_hash, operator_id, conflict_policy,
    outbox_event_id, timeline_event_id
) VALUES
    ('change-limit-1', 'tenant-member', 'conv-member', 'target-1', 'JOIN', 'OUTBOX_ENQUEUED', 'idem-limit-1', 'hash-1', 'owner-1', 'REJECT', 'event-limit-1', 'event-limit-1'),
    ('change-limit-2', 'tenant-member', 'conv-member', 'target-2', 'JOIN', 'OUTBOX_ENQUEUED', 'idem-limit-2', 'hash-2', 'owner-1', 'REJECT', 'event-limit-2', 'event-limit-2');
INSERT INTO message_outbox (
    event_id, tenant_id, conversation_id, aggregate_version, event_type,
    event_version, partition_key, mapping_version, correlation_id, causation_id,
    producer, payload_json, trace_id, status, published_at
) VALUES
    ('event-limit-1', 'tenant-member', 'conv-member', 1, 'conversation.member.joined.v1', '1', 'tenant-member:conv-member', '1', 'c1', 'c1', 'conversation-service', '{}'::jsonb, 'trace-1', 'PUBLISHED', now()),
    ('event-limit-2', 'tenant-member', 'conv-member', 2, 'conversation.member.joined.v1', '1', 'tenant-member:conv-member', '1', 'c2', 'c2', 'conversation-service', '{}'::jsonb, 'trace-2', 'PUBLISHED', now());
`); err != nil {
		t.Fatalf("seed limit data: %v", err)
	}

	stats, err := NewRepository(pool).MarkPublishedMemberChanges(ctx, 1)
	if err != nil {
		t.Fatalf("mark limit: %v", err)
	}
	if stats.Advanced != 1 {
		t.Fatalf("expected one advance, got %+v", stats)
	}
	stats, err = NewRepository(pool).MarkPublishedMemberChanges(ctx, 1)
	if err != nil {
		t.Fatalf("mark second limit: %v", err)
	}
	if stats.Advanced != 1 {
		t.Fatalf("expected second one advance, got %+v", stats)
	}
}

func TestRepositoryMarkPublishedMemberChangesRejectsMismatchedOutboxRows(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `
INSERT INTO member_change_saga (
    change_id, tenant_id, conversation_id, user_id, change_type, status,
    idempotency_key, command_hash, operator_id, conflict_policy,
    outbox_event_id, timeline_event_id
) VALUES
    ('change-bad-producer', 'tenant-member', 'conv-member', 'target-1', 'JOIN', 'OUTBOX_ENQUEUED', 'idem-bad-producer', 'hash-bad-producer', 'owner-1', 'REJECT', 'event-bad-producer', 'event-bad-producer'),
    ('change-bad-type', 'tenant-member', 'conv-member', 'target-2', 'JOIN', 'OUTBOX_ENQUEUED', 'idem-bad-type', 'hash-bad-type', 'owner-1', 'REJECT', 'event-bad-type', 'event-bad-type'),
    ('change-bad-conversation', 'tenant-member', 'conv-member', 'target-3', 'JOIN', 'OUTBOX_ENQUEUED', 'idem-bad-conversation', 'hash-bad-conversation', 'owner-1', 'REJECT', 'event-bad-conversation', 'event-bad-conversation'),
    ('change-good', 'tenant-member', 'conv-member', 'target-4', 'JOIN', 'OUTBOX_ENQUEUED', 'idem-good', 'hash-good', 'owner-1', 'REJECT', 'event-good', 'event-good');
INSERT INTO message_outbox (
    event_id, tenant_id, conversation_id, aggregate_version, event_type,
    event_version, partition_key, mapping_version, correlation_id, causation_id,
    producer, payload_json, trace_id, status, published_at
) VALUES
    ('event-bad-producer', 'tenant-member', 'conv-member', 1, 'conversation.member.joined.v1', '1', 'tenant-member:conv-member', '1', 'c1', 'c1', 'message-service', '{}'::jsonb, 'trace-1', 'PUBLISHED', now()),
    ('event-bad-type', 'tenant-member', 'conv-member', 2, 'message.persisted.v1', '1', 'tenant-member:conv-member', '1', 'c2', 'c2', 'conversation-service', '{}'::jsonb, 'trace-2', 'PUBLISHED', now()),
    ('event-bad-conversation', 'tenant-member', 'conv-other', 3, 'conversation.member.joined.v1', '1', 'tenant-member:conv-other', '1', 'c3', 'c3', 'conversation-service', '{}'::jsonb, 'trace-3', 'PUBLISHED', now()),
    ('event-good', 'tenant-member', 'conv-member', 4, 'conversation.member.joined.v1', '1', 'tenant-member:conv-member', '1', 'c4', 'c4', 'conversation-service', '{}'::jsonb, 'trace-4', 'PUBLISHED', now());
`); err != nil {
		t.Fatalf("seed mismatched published outbox rows: %v", err)
	}

	stats, err := NewRepository(pool).MarkPublishedMemberChanges(ctx, 100)
	if err != nil {
		t.Fatalf("mark with mismatched outbox rows: %v", err)
	}
	if stats.Advanced != 1 {
		t.Fatalf("expected only valid outbox row to advance, got %+v", stats)
	}
	assertMemberChangeStatus(t, ctx, pool, "change-bad-producer", types.MemberChangeStatusOutboxEnqueued, false)
	assertMemberChangeStatus(t, ctx, pool, "change-bad-type", types.MemberChangeStatusOutboxEnqueued, false)
	assertMemberChangeStatus(t, ctx, pool, "change-bad-conversation", types.MemberChangeStatusOutboxEnqueued, false)
	assertMemberChangeStatus(t, ctx, pool, "change-good", types.MemberChangeStatusDone, true)
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

func seedMemberChangeConversation(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
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
}

func assertMemberChangeStatus(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	changeID types.ChangeID,
	want types.MemberChangeStatus,
	wantCompleted bool,
) {
	t.Helper()
	var status types.MemberChangeStatus
	var completedAt sql.NullTime
	if err := pool.QueryRow(ctx, `
SELECT status, completed_at
FROM member_change_saga
WHERE change_id = $1
`, changeID).Scan(&status, &completedAt); err != nil {
		t.Fatalf("query member change status: %v", err)
	}
	if status != want || completedAt.Valid != wantCompleted {
		t.Fatalf("unexpected member change state: status=%s completed=%v", status, completedAt)
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
