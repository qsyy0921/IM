package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

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

	roleFirstPage, err := repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       2,
		Sort:           types.ConversationMemberListSortRoleUserIDAsc,
	})
	if err != nil {
		t.Fatalf("list role-sorted first page: %v", err)
	}
	if len(roleFirstPage.Members) != 2 ||
		roleFirstPage.Members[0].UserID != "owner-1" ||
		roleFirstPage.Members[1].UserID != "admin-1" ||
		roleFirstPage.NextPageToken == "" {
		t.Fatalf("unexpected role-sorted first page: %+v", roleFirstPage)
	}
	roleSecondPage, err := repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       2,
		PageToken:      roleFirstPage.NextPageToken,
		Sort:           types.ConversationMemberListSortRoleUserIDAsc,
	})
	if err != nil {
		t.Fatalf("list role-sorted second page: %v", err)
	}
	if len(roleSecondPage.Members) != 2 ||
		roleSecondPage.Members[0].UserID != "member-1" ||
		roleSecondPage.Members[1].UserID != "member-2" ||
		roleSecondPage.NextPageToken != "" {
		t.Fatalf("unexpected role-sorted second page: %+v", roleSecondPage)
	}
	_, err = repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       2,
		PageToken:      roleFirstPage.NextPageToken,
		Sort:           types.ConversationMemberListSortUserIDAsc,
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor when sort changes, got %v", err)
	}

	privilegedFirstPage, err := repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       1,
		RoleFilters: []types.MemberRole{
			types.MemberRoleOwner,
			types.MemberRoleAdmin,
		},
	})
	if err != nil {
		t.Fatalf("list first privileged members page: %v", err)
	}
	if len(privilegedFirstPage.Members) != 1 ||
		privilegedFirstPage.Members[0].UserID != "admin-1" ||
		privilegedFirstPage.NextPageToken == "" {
		t.Fatalf("unexpected first privileged page: %+v", privilegedFirstPage)
	}
	privilegedSecondPage, err := repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       1,
		PageToken:      privilegedFirstPage.NextPageToken,
		RoleFilters: []types.MemberRole{
			types.MemberRoleAdmin,
			types.MemberRoleOwner,
		},
	})
	if err != nil {
		t.Fatalf("list second privileged members page with reversed filters: %v", err)
	}
	if len(privilegedSecondPage.Members) != 1 ||
		privilegedSecondPage.Members[0].UserID != "owner-1" ||
		privilegedSecondPage.NextPageToken != "" {
		t.Fatalf("unexpected second privileged page: %+v", privilegedSecondPage)
	}
	_, err = repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       1,
		PageToken:      privilegedFirstPage.NextPageToken,
		RoleFilters:    []types.MemberRole{types.MemberRoleAdmin},
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor when role_filters changes, got %v", err)
	}
	privilegedAdminOnly, err := repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       10,
		RoleFilter:     types.MemberRoleAdmin,
		RoleFilters: []types.MemberRole{
			types.MemberRoleOwner,
			types.MemberRoleAdmin,
		},
	})
	if err != nil {
		t.Fatalf("list combined legacy and multi-role filters: %v", err)
	}
	if len(privilegedAdminOnly.Members) != 1 ||
		privilegedAdminOnly.Members[0].UserID != "admin-1" ||
		privilegedAdminOnly.Members[0].Role != types.MemberRoleAdmin {
		t.Fatalf("unexpected combined role filters: %+v", privilegedAdminOnly.Members)
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

	prefixFirstPage, err := repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       1,
		UserIDPrefix:   "member-",
	})
	if err != nil {
		t.Fatalf("list first prefix page: %v", err)
	}
	if len(prefixFirstPage.Members) != 1 ||
		prefixFirstPage.Members[0].UserID != "member-1" ||
		prefixFirstPage.NextPageToken == "" {
		t.Fatalf("unexpected first prefix page: %+v", prefixFirstPage)
	}
	prefixSecondPage, err := repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       1,
		PageToken:      prefixFirstPage.NextPageToken,
		UserIDPrefix:   "member-",
	})
	if err != nil {
		t.Fatalf("list second prefix page: %v", err)
	}
	if len(prefixSecondPage.Members) != 1 ||
		prefixSecondPage.Members[0].UserID != "member-2" ||
		prefixSecondPage.NextPageToken != "" {
		t.Fatalf("unexpected second prefix page: %+v", prefixSecondPage)
	}
	_, err = repository.ListConversationMembers(ctx, types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-list",
			UserID:   "owner-1",
		},
		ConversationID: "conv-list",
		PageSize:       1,
		PageToken:      prefixFirstPage.NextPageToken,
		UserIDPrefix:   "owner-",
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor when user_id_prefix changes, got %v", err)
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
