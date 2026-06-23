package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func (r *Repository) GetConversationProfile(
	ctx context.Context,
	command types.GetConversationProfileCommand,
) (types.ConversationProfileResult, error) {
	if r.pool == nil {
		return types.ConversationProfileResult{}, types.NewDBReadFailed("repository is not configured")
	}
	if err := command.Validate(); err != nil {
		return types.ConversationProfileResult{}, err
	}
	row := r.pool.QueryRow(ctx, `
SELECT
    c.tenant_id,
    c.conversation_id,
    c.conversation_type,
    c.status,
    c.title,
    c.avatar_uri,
    c.announcement,
    c.profile_version,
    c.member_version,
    c.permission_version,
    c.profile_updated_at,
    COALESCE(auth_member.status, '')
FROM conversations c
LEFT JOIN conversation_members auth_member
  ON auth_member.tenant_id = c.tenant_id
 AND auth_member.conversation_id = c.conversation_id
 AND auth_member.user_id = $3
WHERE c.tenant_id = $1
  AND c.conversation_id = $2
`, command.AuthContext.TenantID, command.ConversationID, command.AuthContext.UserID)
	var result types.ConversationProfileResult
	var conversationStatus types.ConversationStatus
	var authStatus types.MemberStatus
	if err := row.Scan(
		&result.TenantID,
		&result.ConversationID,
		&result.ConversationType,
		&conversationStatus,
		&result.Title,
		&result.AvatarURI,
		&result.Announcement,
		&result.ProfileVersion,
		&result.MemberVersion,
		&result.PermissionVersion,
		&result.UpdatedAt,
		&authStatus,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.ConversationProfileResult{}, types.NewConversationNotFound("conversation not found")
		}
		return types.ConversationProfileResult{}, types.NewDBReadFailed(err.Error())
	}
	if conversationStatus != types.ConversationStatusActive {
		return types.ConversationProfileResult{}, types.NewConversationNotFound("conversation not found")
	}
	if authStatus != types.MemberStatusActive {
		return types.ConversationProfileResult{}, types.NewMemberNotActive("conversation member is not active")
	}
	return result, nil
}

func (r *Repository) UpdateConversationProfile(
	ctx context.Context,
	command types.UpdateConversationProfileCommand,
) (types.ConversationProfileResult, error) {
	if r.pool == nil {
		return types.ConversationProfileResult{}, types.NewDBWriteFailed("repository is not configured")
	}
	if err := command.Validate(); err != nil {
		return types.ConversationProfileResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.ConversationProfileResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var current types.ConversationProfileResult
	var conversationStatus types.ConversationStatus
	var authRole types.MemberRole
	var authStatus types.MemberStatus
	if err := tx.QueryRow(ctx, `
SELECT
    c.tenant_id,
    c.conversation_id,
    c.conversation_type,
    c.status,
    c.profile_version,
    c.member_version,
    c.permission_version,
    COALESCE(auth_member.role, ''),
    COALESCE(auth_member.status, '')
FROM conversations c
LEFT JOIN conversation_members auth_member
  ON auth_member.tenant_id = c.tenant_id
 AND auth_member.conversation_id = c.conversation_id
 AND auth_member.user_id = $3
WHERE c.tenant_id = $1
  AND c.conversation_id = $2
FOR UPDATE OF c
`, command.AuthContext.TenantID, command.ConversationID, command.AuthContext.UserID).Scan(
		&current.TenantID,
		&current.ConversationID,
		&current.ConversationType,
		&conversationStatus,
		&current.ProfileVersion,
		&current.MemberVersion,
		&current.PermissionVersion,
		&authRole,
		&authStatus,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.ConversationProfileResult{}, types.NewConversationNotFound("conversation not found")
		}
		return types.ConversationProfileResult{}, types.NewDBReadFailed(err.Error())
	}
	if conversationStatus != types.ConversationStatusActive {
		return types.ConversationProfileResult{}, types.NewConversationNotFound("conversation not found")
	}
	if current.ConversationType != types.ConversationTypeGroup {
		return types.ConversationProfileResult{}, types.NewInvalidArgument("only group conversation profile can be updated")
	}
	if authStatus != types.MemberStatusActive {
		return types.ConversationProfileResult{}, types.NewMemberNotActive("conversation member is not active")
	}
	if authRole != types.MemberRoleOwner && authRole != types.MemberRoleAdmin {
		return types.ConversationProfileResult{}, types.NewPermissionDenied("conversation profile update requires owner or admin")
	}
	if command.ExpectedProfileVersion > 0 && command.ExpectedProfileVersion != current.ProfileVersion {
		return types.ConversationProfileResult{}, types.NewProfileConflict("profile version mismatch")
	}

	updatedAt := r.now()
	if err := tx.QueryRow(ctx, `
UPDATE conversations
SET title = $3,
    avatar_uri = $4,
    announcement = $5,
    profile_version = profile_version + 1,
    profile_updated_at = $6,
    updated_at = $6
WHERE tenant_id = $1
  AND conversation_id = $2
RETURNING
    tenant_id,
    conversation_id,
    conversation_type,
    title,
    avatar_uri,
    announcement,
    profile_version,
    member_version,
    permission_version,
    profile_updated_at
`, command.AuthContext.TenantID, command.ConversationID, command.NormalizedTitle(), command.NormalizedAvatarURI(), command.NormalizedAnnouncement(), updatedAt).Scan(
		&current.TenantID,
		&current.ConversationID,
		&current.ConversationType,
		&current.Title,
		&current.AvatarURI,
		&current.Announcement,
		&current.ProfileVersion,
		&current.MemberVersion,
		&current.PermissionVersion,
		&current.UpdatedAt,
	); err != nil {
		return types.ConversationProfileResult{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ConversationProfileResult{}, types.NewDBWriteFailed(err.Error())
	}
	return current, nil
}
