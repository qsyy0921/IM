package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestProjectionRepositoryProjectsConversationMembersIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	repository := NewProjectionRepository(pool)

	joined := conversationMemberCommand(types.TimelineEventConversationMemberJoined, "member-joined-1", "alice", types.ConversationMemberRoleMember, types.ConversationMemberStatusActive, 1, 7)
	result, err := repository.ProjectConversationMemberEvent(ctx, joined)
	if err != nil {
		t.Fatalf("project joined member: %v", err)
	}
	if result.ProjectedMembers != 1 || result.Ignored {
		t.Fatalf("expected one projected member, got %+v", result)
	}
	assertConversationMember(t, ctx, pool, "alice", types.ConversationMemberRoleMember, types.ConversationMemberStatusActive, 1, 7)

	stale := conversationMemberCommand(types.TimelineEventConversationMemberRoleChanged, "member-stale-1", "alice", types.ConversationMemberRoleAdmin, types.ConversationMemberStatusActive, 1, 8)
	result, err = repository.ProjectConversationMemberEvent(ctx, stale)
	if err != nil {
		t.Fatalf("project stale member: %v", err)
	}
	if result.ProjectedMembers != 0 {
		t.Fatalf("expected stale member event to be no-op, got %+v", result)
	}
	assertConversationMember(t, ctx, pool, "alice", types.ConversationMemberRoleMember, types.ConversationMemberStatusActive, 1, 7)

	left := conversationMemberCommand(types.TimelineEventConversationMemberLeft, "member-left-1", "alice", types.ConversationMemberRoleMember, types.ConversationMemberStatusLeft, 2, 9)
	result, err = repository.ProjectConversationMemberEvent(ctx, left)
	if err != nil {
		t.Fatalf("project left member: %v", err)
	}
	if result.ProjectedMembers != 1 {
		t.Fatalf("expected left event to update member, got %+v", result)
	}
	assertConversationMember(t, ctx, pool, "alice", types.ConversationMemberRoleMember, types.ConversationMemberStatusLeft, 2, 9)
	assertPolicyCheckpoint(t, ctx, pool, "policy-timeline-test", types.TimelineEventConversationMemberLeft, 12)
}

func TestProjectionRepositoryProjectsOwnerTransferredIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	repository := NewProjectionRepository(pool)

	result, err := repository.ProjectConversationMemberEvent(ctx, types.ProjectConversationMemberEventCommand{
		TenantID:             "tenant-policy",
		EventID:              "owner-transfer-1",
		EventType:            types.TimelineEventConversationMemberOwnerTransferred,
		ConversationID:       "conv-policy",
		ConversationSeq:      3,
		PreviousOwnerUserID:  "alice",
		PreviousOwnerNewRole: types.ConversationMemberRoleAdmin,
		PreviousOwnerStatus:  types.ConversationMemberStatusActive,
		NewOwnerUserID:       "bob",
		NewOwnerNewRole:      types.ConversationMemberRoleOwner,
		NewOwnerStatus:       types.ConversationMemberStatusActive,
		MemberVersion:        4,
		PermissionVersion:    11,
		ConsumerGroup:        "policy-timeline-test",
		Topic:                types.TimelineEventConversationMemberOwnerTransferred,
		PartitionID:          3,
		OffsetValue:          20,
	})
	if err != nil {
		t.Fatalf("project owner transfer: %v", err)
	}
	if result.ProjectedMembers != 2 {
		t.Fatalf("expected two projected owners, got %+v", result)
	}
	assertConversationMember(t, ctx, pool, "alice", types.ConversationMemberRoleAdmin, types.ConversationMemberStatusActive, 4, 11)
	assertConversationMember(t, ctx, pool, "bob", types.ConversationMemberRoleOwner, types.ConversationMemberStatusActive, 4, 11)
	assertPolicyCheckpoint(t, ctx, pool, "policy-timeline-test", types.TimelineEventConversationMemberOwnerTransferred, 20)
}

func TestProjectionRepositoryIgnoresMessageTimelineEventsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	repository := NewProjectionRepository(pool)

	result, err := repository.ProjectConversationMemberEvent(ctx, types.ProjectConversationMemberEventCommand{
		TenantID:        "tenant-policy",
		EventID:         "message-1",
		EventType:       types.TimelineEventMessagePersisted,
		ConversationID:  "conv-policy",
		ConversationSeq: 5,
		ConsumerGroup:   "policy-timeline-test",
		Topic:           types.TimelineEventMessagePersisted,
		PartitionID:     3,
		OffsetValue:     30,
	})
	if err != nil {
		t.Fatalf("project ignored message event: %v", err)
	}
	if !result.Ignored || result.ProjectedMembers != 0 {
		t.Fatalf("expected message event to be ignored, got %+v", result)
	}
	assertPolicyCheckpoint(t, ctx, pool, "policy-timeline-test", types.TimelineEventMessagePersisted, 30)
}

func conversationMemberCommand(
	eventType string,
	eventID string,
	userID string,
	role string,
	status string,
	memberVersion int64,
	permissionVersion int64,
) types.ProjectConversationMemberEventCommand {
	return types.ProjectConversationMemberEventCommand{
		TenantID:          "tenant-policy",
		EventID:           eventID,
		EventType:         eventType,
		ConversationID:    "conv-policy",
		ConversationSeq:   10 + memberVersion,
		MemberUserID:      types.UserID(userID),
		MemberRole:        role,
		MemberStatus:      status,
		MemberVersion:     memberVersion,
		PermissionVersion: permissionVersion,
		ConsumerGroup:     "policy-timeline-test",
		Topic:             eventType,
		PartitionID:       3,
		OffsetValue:       10 + memberVersion,
	}
}

func assertConversationMember(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	userID string,
	role string,
	status string,
	memberVersion int64,
	permissionVersion int64,
) {
	t.Helper()
	var gotRole string
	var gotStatus string
	var gotMemberVersion int64
	var gotPermissionVersion int64
	err := pool.QueryRow(ctx, `
SELECT role, status, member_version, permission_version
FROM policy_conversation_members_projection
WHERE tenant_id = 'tenant-policy'
  AND conversation_id = 'conv-policy'
  AND user_id = $1
`, userID).Scan(&gotRole, &gotStatus, &gotMemberVersion, &gotPermissionVersion)
	if err != nil {
		t.Fatalf("query projected conversation member %s: %v", userID, err)
	}
	if gotRole != role || gotStatus != status || gotMemberVersion != memberVersion || gotPermissionVersion != permissionVersion {
		t.Fatalf(
			"unexpected projected member %s: got %s/%s/%d/%d want %s/%s/%d/%d",
			userID,
			gotRole,
			gotStatus,
			gotMemberVersion,
			gotPermissionVersion,
			role,
			status,
			memberVersion,
			permissionVersion,
		)
	}
}
