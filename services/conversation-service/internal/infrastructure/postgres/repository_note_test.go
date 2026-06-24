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

func TestRepositoryCreateConversationNoteIntegration(t *testing.T) {
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
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
    ('tenant-note', 'conv-note', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 5, 7, 'local'),
    ('tenant-note', 'conv-deleted', 'GROUP', 'DELETED', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 5, 7, 'local');
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, member_version, permission_version
) VALUES
    ('tenant-note', 'conv-note', 'owner-1', 'OWNER', 'ACTIVE', 5, 7),
    ('tenant-note', 'conv-note', 'member-1', 'MEMBER', 'ACTIVE', 5, 7),
    ('tenant-note', 'conv-note', 'left-1', 'MEMBER', 'LEFT', 5, 7),
    ('tenant-note', 'conv-deleted', 'owner-1', 'OWNER', 'ACTIVE', 5, 7);
`)
	if err != nil {
		t.Fatalf("seed conversation notes: %v", err)
	}

	repository := NewRepository(pool)
	command := types.CreateConversationNoteCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-note",
			UserID:   "member-1",
		},
		ConversationID:   "conv-note",
		Body:             "  approved summary note  ",
		IdempotencyKey:   "proposal-1:approval-1",
		SourceToolName:   "conversation.note.create",
		SourceProposalID: "proposal-1",
		SourceApprovalID: "approval-1",
	}
	created, err := repository.CreateConversationNote(ctx, command)
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if created.NoteID == "" ||
		created.Body != "approved summary note" ||
		created.AuthorUserID != "member-1" ||
		created.SourceToolName != "conversation.note.create" ||
		created.SourceProposalID != "proposal-1" ||
		created.SourceApprovalID != "approval-1" ||
		created.IdempotentReplay {
		t.Fatalf("unexpected created note: %+v", created)
	}

	replayed, err := repository.CreateConversationNote(ctx, command)
	if err != nil {
		t.Fatalf("replay note: %v", err)
	}
	if !replayed.IdempotentReplay || replayed.NoteID != created.NoteID || replayed.Body != created.Body {
		t.Fatalf("expected idempotent replay of %+v, got %+v", created, replayed)
	}

	if _, err := repository.CreateConversationNote(ctx, types.CreateConversationNoteCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-note",
			UserID:   "left-1",
		},
		ConversationID: "conv-note",
		Body:           "left member should not write",
		IdempotencyKey: "left-key",
	}); !errors.Is(err, types.ErrMemberNotActive) {
		t.Fatalf("expected member not active, got %v", err)
	}

	if _, err := repository.CreateConversationNote(ctx, types.CreateConversationNoteCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-note",
			UserID:   "owner-1",
		},
		ConversationID: "conv-deleted",
		Body:           "deleted conversation should not write",
		IdempotencyKey: "deleted-key",
	}); !errors.Is(err, types.ErrConversationNotFound) {
		t.Fatalf("expected conversation not found, got %v", err)
	}
}
