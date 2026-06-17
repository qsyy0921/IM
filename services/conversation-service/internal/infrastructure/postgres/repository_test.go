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
