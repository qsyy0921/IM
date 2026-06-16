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

func TestRepositoryAuditMemberWindowsIntegration(t *testing.T) {
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
    ('tenant-window', 'conv-active', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 1, 10, 20, 'local'),
    ('tenant-window', 'conv-no-owner', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 1, 10, 20, 'local'),
    ('tenant-window', 'conv-multi-owner', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 1, 10, 20, 'local'),
    ('tenant-window', 'conv-archived', 'GROUP', 'ARCHIVED', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 1, 10, 20, 'local');

INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, join_seq, leave_seq,
    member_version, permission_version, updated_at
) VALUES
    ('tenant-window', 'conv-active', 'active-missing-join', 'MEMBER', 'ACTIVE', NULL, NULL, 9, 19, now() - interval '1 minute'),
    ('tenant-window', 'conv-active', 'active-with-leave', 'MEMBER', 'ACTIVE', 2, 3, 9, 19, now() - interval '2 minutes'),
    ('tenant-window', 'conv-active', 'left-missing-leave', 'MEMBER', 'LEFT', 4, NULL, 9, 19, now() - interval '3 minutes'),
    ('tenant-window', 'conv-active', 'leave-before-join', 'MEMBER', 'LEFT', 8, 7, 9, 19, now() - interval '4 minutes'),
    ('tenant-window', 'conv-active', 'version-ahead', 'MEMBER', 'ACTIVE', 5, NULL, 11, 19, now() - interval '5 minutes'),
    ('tenant-window', 'conv-active', 'permission-ahead', 'MEMBER', 'ACTIVE', 6, NULL, 9, 21, now() - interval '6 minutes'),
    ('tenant-window', 'conv-archived', 'active-archived', 'OWNER', 'ACTIVE', 1, NULL, 9, 19, now() - interval '7 minutes'),
    ('tenant-window', 'conv-active', 'healthy-member', 'MEMBER', 'ACTIVE', 1, NULL, 9, 19, now() - interval '8 minutes'),
    ('tenant-window', 'conv-active', 'healthy-owner', 'OWNER', 'ACTIVE', 1, NULL, 9, 19, now() - interval '9 minutes'),
    ('tenant-window', 'conv-no-owner', 'member-only', 'MEMBER', 'ACTIVE', 1, NULL, 9, 19, now() - interval '10 minutes'),
    ('tenant-window', 'conv-multi-owner', 'owner-a', 'OWNER', 'ACTIVE', 1, NULL, 9, 19, now() - interval '11 minutes'),
    ('tenant-window', 'conv-multi-owner', 'owner-b', 'OWNER', 'ACTIVE', 2, NULL, 10, 20, now() - interval '12 minutes');
`)
	if err != nil {
		t.Fatalf("seed member windows: %v", err)
	}

	repository := NewRepository(pool)
	rows, err := repository.AuditMemberWindows(ctx, MemberWindowAuditOptions{
		TenantID:       "tenant-window",
		ConversationID: "conv-active",
		Limit:          20,
	})
	if err != nil {
		t.Fatalf("audit member windows: %v", err)
	}
	if len(rows) != 6 {
		t.Fatalf("expected six active-conversation issues, got %d: %+v", len(rows), rows)
	}
	issueByUser := make(map[string]string, len(rows))
	for _, row := range rows {
		issueByUser[row.UserID] = row.IssueClass
		if row.UserID == "healthy-member" {
			t.Fatalf("healthy member should not be reported: %+v", row)
		}
	}
	for userID, wantIssue := range map[string]string{
		"active-missing-join": "ACTIVE_WITHOUT_JOIN_SEQ",
		"active-with-leave":   "ACTIVE_WITH_LEAVE_SEQ",
		"left-missing-leave":  "INACTIVE_WITHOUT_LEAVE_SEQ",
		"leave-before-join":   "LEAVE_BEFORE_JOIN",
		"version-ahead":       "MEMBER_VERSION_AHEAD_CONVERSATION",
		"permission-ahead":    "PERMISSION_VERSION_AHEAD_CONVERSATION",
	} {
		if got := issueByUser[userID]; got != wantIssue {
			t.Fatalf("issue for %s = %q, want %q", userID, got, wantIssue)
		}
	}

	updatedAfter := time.Now().Add(-90 * time.Second)
	updatedBefore := time.Now().Add(30 * time.Second)
	windowed, err := repository.AuditMemberWindows(ctx, MemberWindowAuditOptions{
		TenantID:       "tenant-window",
		ConversationID: "conv-active",
		UpdatedAfter:   &updatedAfter,
		UpdatedBefore:  &updatedBefore,
		Limit:          20,
	})
	if err != nil {
		t.Fatalf("audit member windows by updated window: %v", err)
	}
	if len(windowed) != 1 || windowed[0].UserID != "active-missing-join" {
		t.Fatalf("unexpected updated window rows: %+v", windowed)
	}
	emptyAfter := time.Now().Add(time.Hour)
	emptyRows, err := repository.AuditMemberWindows(ctx, MemberWindowAuditOptions{
		TenantID:       "tenant-window",
		ConversationID: "conv-active",
		UpdatedAfter:   &emptyAfter,
		Limit:          20,
	})
	if err != nil {
		t.Fatalf("audit member windows by empty updated window: %v", err)
	}
	if len(emptyRows) != 0 {
		t.Fatalf("expected empty updated window rows, got %+v", emptyRows)
	}
	if _, err := repository.AuditMemberWindows(ctx, MemberWindowAuditOptions{
		UpdatedAfter:  &updatedBefore,
		UpdatedBefore: &updatedBefore,
	}); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid updated window error, got %v", err)
	}

	filtered, err := repository.AuditMemberWindows(ctx, MemberWindowAuditOptions{
		TenantID:   "tenant-window",
		IssueClass: "active_member_in_inactive_conversation",
		Role:       "owner",
		Status:     "active",
	})
	if err != nil {
		t.Fatalf("audit member windows by issue: %v", err)
	}
	if len(filtered) != 1 || filtered[0].UserID != "active-archived" {
		t.Fatalf("unexpected filtered result: %+v", filtered)
	}

	withoutOwner, err := repository.AuditMemberWindows(ctx, MemberWindowAuditOptions{
		TenantID:       "tenant-window",
		ConversationID: "conv-no-owner",
		IssueClass:     "active_conversation_without_owner",
	})
	if err != nil {
		t.Fatalf("audit active conversation without owner: %v", err)
	}
	if len(withoutOwner) != 1 ||
		withoutOwner[0].ConversationID != "conv-no-owner" ||
		withoutOwner[0].UserID != "" ||
		withoutOwner[0].IssueClass != "ACTIVE_CONVERSATION_WITHOUT_OWNER" {
		t.Fatalf("unexpected without-owner result: %+v", withoutOwner)
	}

	multipleOwners, err := repository.AuditMemberWindows(ctx, MemberWindowAuditOptions{
		TenantID:       "tenant-window",
		ConversationID: "conv-multi-owner",
		IssueClass:     "active_conversation_with_multiple_owners",
	})
	if err != nil {
		t.Fatalf("audit active conversation with multiple owners: %v", err)
	}
	if len(multipleOwners) != 2 {
		t.Fatalf("expected two multiple-owner rows, got %+v", multipleOwners)
	}
	for _, row := range multipleOwners {
		if row.Role != "OWNER" ||
			row.Status != "ACTIVE" ||
			row.IssueClass != "ACTIVE_CONVERSATION_WITH_MULTIPLE_OWNERS" {
			t.Fatalf("unexpected multiple-owner row: %+v", row)
		}
	}

	if _, err := repository.AuditMemberWindows(ctx, MemberWindowAuditOptions{IssueClass: "unknown"}); err == nil {
		t.Fatalf("expected unsupported issue class to fail")
	}
	if _, err := repository.AuditMemberWindows(ctx, MemberWindowAuditOptions{Role: "boss"}); err == nil {
		t.Fatalf("expected unsupported role to fail")
	}
	if _, err := repository.AuditMemberWindows(ctx, MemberWindowAuditOptions{Status: "pending"}); err == nil {
		t.Fatalf("expected unsupported status to fail")
	}
}
