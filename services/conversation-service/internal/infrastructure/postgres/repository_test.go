package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

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

func TestRepositoryConversationProfileAnnouncementIntegration(t *testing.T) {
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
    tenant_id, conversation_id, conversation_type, status, title, avatar_uri, announcement,
    conversation_mode, fanout_mode, fanout_policy_version, profile_version,
    member_version, permission_version, current_seq_shard
) VALUES ('tenant-profile', 'conv-profile', 'GROUP', 'ACTIVE', '旧群', 'media://asset/old', '旧公告',
    'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 2, 5, 7, 'local');
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, member_version, permission_version
) VALUES
    ('tenant-profile', 'conv-profile', 'owner-1', 'OWNER', 'ACTIVE', 5, 7),
    ('tenant-profile', 'conv-profile', 'admin-1', 'ADMIN', 'ACTIVE', 5, 7),
    ('tenant-profile', 'conv-profile', 'member-1', 'MEMBER', 'ACTIVE', 5, 7);
`)
	if err != nil {
		t.Fatalf("seed conversation profile: %v", err)
	}

	repository := NewRepository(pool)
	profile, err := repository.GetConversationProfile(ctx, types.GetConversationProfileCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-profile",
			UserID:   "owner-1",
		},
		ConversationID: "conv-profile",
	})
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if profile.Title != "旧群" || profile.AvatarURI != "media://asset/old" || profile.Announcement != "旧公告" {
		t.Fatalf("unexpected initial profile: %+v", profile)
	}

	updated, err := repository.UpdateConversationProfile(ctx, types.UpdateConversationProfileCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-profile",
			UserID:   "admin-1",
		},
		ConversationID:         "conv-profile",
		Title:                  " 新群 ",
		AvatarURI:              " media://asset/new ",
		Announcement:           " 新公告 ",
		ExpectedProfileVersion: profile.ProfileVersion,
	})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.Title != "新群" || updated.AvatarURI != "media://asset/new" || updated.Announcement != "新公告" {
		t.Fatalf("unexpected updated profile: %+v", updated)
	}
	if updated.ProfileVersion != profile.ProfileVersion+1 {
		t.Fatalf("expected profile version %d, got %d", profile.ProfileVersion+1, updated.ProfileVersion)
	}

	if _, err := repository.UpdateConversationProfile(ctx, types.UpdateConversationProfileCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-profile",
			UserID:   "member-1",
		},
		ConversationID:         "conv-profile",
		Title:                  "普通成员修改",
		Announcement:           "不应写入",
		ExpectedProfileVersion: updated.ProfileVersion,
	}); !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}

	if _, err := repository.UpdateConversationProfile(ctx, types.UpdateConversationProfileCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-profile",
			UserID:   "owner-1",
		},
		ConversationID:         "conv-profile",
		Title:                  "旧版本修改",
		Announcement:           "不应写入",
		ExpectedProfileVersion: profile.ProfileVersion,
	}); !errors.Is(err, types.ErrProfileConflict) {
		t.Fatalf("expected profile conflict, got %v", err)
	}

	var storedAnnouncement string
	if err := pool.QueryRow(ctx, `
SELECT announcement
FROM conversations
WHERE tenant_id = 'tenant-profile'
  AND conversation_id = 'conv-profile'
`).Scan(&storedAnnouncement); err != nil {
		t.Fatalf("query stored announcement: %v", err)
	}
	if storedAnnouncement != "新公告" {
		t.Fatalf("expected stored announcement 新公告, got %q", storedAnnouncement)
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
