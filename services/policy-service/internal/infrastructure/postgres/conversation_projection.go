package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func (repository *ProjectionRepository) ProjectConversationMemberEvent(
	ctx context.Context,
	command types.ProjectConversationMemberEventCommand,
) (types.ProjectConversationMemberEventResult, error) {
	if repository == nil || repository.pool == nil {
		return types.ProjectConversationMemberEventResult{}, types.NewDBWriteFailed("policy projection repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.ProjectConversationMemberEventResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer tx.Rollback(ctx)

	result := types.ProjectConversationMemberEventResult{}
	switch command.EventType {
	case types.TimelineEventMessagePersisted,
		types.TimelineEventMessageEdited,
		types.TimelineEventMessageRevoked,
		types.TimelineEventMessageDeleted,
		types.TimelineEventConversationMemberBoundaryCancelled:
		result.Ignored = true
	case types.TimelineEventConversationMemberJoined,
		types.TimelineEventConversationMemberLeft,
		types.TimelineEventConversationMemberRemoved,
		types.TimelineEventConversationMemberRoleChanged:
		projected, err := upsertConversationMemberProjection(ctx, tx, command.TenantID, command.ConversationID, command.MemberUserID, command.MemberRole, command.MemberStatus, command.MemberVersion, command.PermissionVersion, command.EventID)
		if err != nil {
			return types.ProjectConversationMemberEventResult{}, err
		}
		if projected {
			result.ProjectedMembers = 1
		}
	case types.TimelineEventConversationMemberOwnerTransferred:
		projected, err := upsertConversationMemberProjection(ctx, tx, command.TenantID, command.ConversationID, command.PreviousOwnerUserID, command.PreviousOwnerNewRole, command.PreviousOwnerStatus, command.MemberVersion, command.PermissionVersion, command.EventID)
		if err != nil {
			return types.ProjectConversationMemberEventResult{}, err
		}
		if projected {
			result.ProjectedMembers++
		}
		projected, err = upsertConversationMemberProjection(ctx, tx, command.TenantID, command.ConversationID, command.NewOwnerUserID, command.NewOwnerNewRole, command.NewOwnerStatus, command.MemberVersion, command.PermissionVersion, command.EventID)
		if err != nil {
			return types.ProjectConversationMemberEventResult{}, err
		}
		if projected {
			result.ProjectedMembers++
		}
	default:
		return types.ProjectConversationMemberEventResult{}, types.NewInvalidArgument("unsupported timeline event type")
	}
	if command.ShouldCheckpoint() {
		if err := upsertPolicyKafkaCheckpointValues(ctx, tx, command.ConsumerGroup, command.Topic, command.PartitionID, command.OffsetValue); err != nil {
			return types.ProjectConversationMemberEventResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ProjectConversationMemberEventResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func upsertConversationMemberProjection(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	conversationID types.ConversationID,
	userID types.UserID,
	role string,
	status string,
	memberVersion int64,
	permissionVersion int64,
	eventID string,
) (bool, error) {
	tag, err := tx.Exec(ctx, `
INSERT INTO policy_conversation_members_projection (
    tenant_id,
    conversation_id,
    user_id,
    role,
    status,
    member_version,
    permission_version,
    updated_by_event_id,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (tenant_id, conversation_id, user_id) DO UPDATE
SET role = EXCLUDED.role,
    status = EXCLUDED.status,
    member_version = EXCLUDED.member_version,
    permission_version = EXCLUDED.permission_version,
    updated_by_event_id = EXCLUDED.updated_by_event_id,
    updated_at = now()
WHERE policy_conversation_members_projection.member_version <= EXCLUDED.member_version
`, tenantID, conversationID, userID, role, status, memberVersion, permissionVersion, eventID)
	if err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	return tag.RowsAffected() > 0, nil
}
