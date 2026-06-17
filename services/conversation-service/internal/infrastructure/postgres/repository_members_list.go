package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type listMembersPageToken struct {
	Version      int      `json:"v"`
	UserID       string   `json:"user_id"`
	Role         string   `json:"role,omitempty"`
	Sort         string   `json:"sort,omitempty"`
	RoleFilter   string   `json:"role_filter"`
	RoleFilters  []string `json:"role_filters,omitempty"`
	UserIDPrefix string   `json:"user_id_prefix,omitempty"`
}

func (r *Repository) ListConversationMembers(
	ctx context.Context,
	command types.ListConversationMembersCommand,
) (types.ListConversationMembersResult, error) {
	if r.pool == nil {
		return types.ListConversationMembersResult{}, types.NewDBReadFailed("repository is not configured")
	}
	if err := command.Validate(); err != nil {
		return types.ListConversationMembersResult{}, err
	}
	roleFilters, err := types.NormalizeListMemberRoleFilters(command.RoleFilters)
	if err != nil {
		return types.ListConversationMembersResult{}, err
	}
	sort, err := types.NormalizeConversationMemberListSort(command.Sort)
	if err != nil {
		return types.ListConversationMembersResult{}, err
	}
	roleFilterValues := memberRolesToStrings(roleFilters)
	pageToken, hasPageToken, err := decodeListMembersPageToken(command.PageToken, command.RoleFilter, roleFilterValues, sort, command.UserIDPrefix)
	if err != nil {
		return types.ListConversationMembersResult{}, err
	}

	var conversationStatus types.ConversationStatus
	var authStatus types.MemberStatus
	result := types.ListConversationMembersResult{
		TenantID:       command.AuthContext.TenantID,
		ConversationID: command.ConversationID,
	}
	if err := r.pool.QueryRow(ctx, `
SELECT
    c.status,
    c.member_version,
    c.permission_version,
    COALESCE(auth_member.status, '')
FROM conversations c
LEFT JOIN conversation_members auth_member
  ON auth_member.tenant_id = c.tenant_id
 AND auth_member.conversation_id = c.conversation_id
 AND auth_member.user_id = $3
WHERE c.tenant_id = $1
  AND c.conversation_id = $2
`, command.AuthContext.TenantID, command.ConversationID, command.AuthContext.UserID).Scan(
		&conversationStatus,
		&result.MemberVersion,
		&result.PermissionVersion,
		&authStatus,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.ListConversationMembersResult{}, types.NewConversationNotFound("conversation not found")
		}
		return types.ListConversationMembersResult{}, types.NewDBReadFailed(err.Error())
	}
	if conversationStatus != types.ConversationStatusActive {
		return types.ListConversationMembersResult{}, types.NewConversationNotFound("conversation not found")
	}
	if authStatus != types.MemberStatusActive {
		return types.ListConversationMembersResult{}, types.NewMemberNotActive("conversation member is not active")
	}

	pageSize := command.EffectivePageSize()
	args := []any{
		command.AuthContext.TenantID,
		command.ConversationID,
		command.RoleFilter,
		roleFilterValues,
		command.UserIDPrefix,
		pageSize + 1,
	}
	query := `
SELECT
    user_id,
    role,
    status,
    COALESCE(join_seq, 0),
    COALESCE(leave_seq, 0),
    member_version,
    permission_version,
    updated_at
FROM conversation_members
WHERE tenant_id = $1
  AND conversation_id = $2
  AND status = 'ACTIVE'
  AND ($3 = '' OR role = $3)
  AND (cardinality($4::text[]) = 0 OR role = ANY($4::text[]))
  AND ($5 = '' OR left(user_id, length($5)) = $5)
`
	if hasPageToken {
		switch sort {
		case types.ConversationMemberListSortRoleUserIDAsc:
			query += `  AND (
      CASE role WHEN 'OWNER' THEN 1 WHEN 'ADMIN' THEN 2 ELSE 3 END > $7
      OR (CASE role WHEN 'OWNER' THEN 1 WHEN 'ADMIN' THEN 2 ELSE 3 END = $7 AND user_id > $8)
  )
`
			args = append(args, memberRoleSortWeight(types.MemberRole(pageToken.Role)), pageToken.UserID)
		default:
			query += `  AND user_id > $7
`
			args = append(args, pageToken.UserID)
		}
	}
	switch sort {
	case types.ConversationMemberListSortRoleUserIDAsc:
		query += `ORDER BY CASE role WHEN 'OWNER' THEN 1 WHEN 'ADMIN' THEN 2 ELSE 3 END ASC, user_id ASC
LIMIT $6
`
	default:
		query += `ORDER BY user_id ASC
LIMIT $6
`
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return types.ListConversationMembersResult{}, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	members := make([]types.ConversationMember, 0, pageSize)
	for rows.Next() {
		var member types.ConversationMember
		if err := rows.Scan(
			&member.UserID,
			&member.Role,
			&member.Status,
			&member.JoinSeq,
			&member.LeaveSeq,
			&member.MemberVersion,
			&member.PermissionVersion,
			&member.UpdatedAt,
		); err != nil {
			return types.ListConversationMembersResult{}, types.NewDBReadFailed(err.Error())
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return types.ListConversationMembersResult{}, types.NewDBReadFailed(err.Error())
	}
	if len(members) > pageSize {
		page := members[:pageSize]
		nextToken, err := encodeListMembersPageToken(page[len(page)-1], command.RoleFilter, roleFilterValues, sort, command.UserIDPrefix)
		if err != nil {
			return types.ListConversationMembersResult{}, types.NewDBReadFailed(err.Error())
		}
		result.Members = page
		result.NextPageToken = nextToken
		return result, nil
	}
	result.Members = members
	return result, nil
}

func decodeListMembersPageToken(token string, roleFilter types.MemberRole, roleFilters []string, sort string, userIDPrefix string) (listMembersPageToken, bool, error) {
	if token == "" {
		return listMembersPageToken{}, false, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return listMembersPageToken{}, false, types.NewInvalidArgument("page_token is invalid")
	}
	var decoded listMembersPageToken
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return listMembersPageToken{}, false, types.NewInvalidArgument("page_token is invalid")
	}
	if decoded.Version == 1 {
		if sort != types.ConversationMemberListSortUserIDAsc || roleFilter != "" || len(roleFilters) != 0 || userIDPrefix != "" || decoded.UserID == "" {
			return listMembersPageToken{}, false, types.NewInvalidArgument("page_token is invalid")
		}
		return decoded, true, nil
	}
	if decoded.Version == 2 {
		if sort != types.ConversationMemberListSortUserIDAsc ||
			decoded.UserID == "" ||
			decoded.RoleFilter != string(roleFilter) ||
			len(roleFilters) != 0 ||
			userIDPrefix != "" {
			return listMembersPageToken{}, false, types.NewInvalidArgument("page_token is invalid")
		}
		return decoded, true, nil
	}
	if decoded.Version == 3 {
		if sort != types.ConversationMemberListSortUserIDAsc ||
			decoded.UserID == "" ||
			decoded.RoleFilter != string(roleFilter) ||
			!sameStringSlice(decoded.RoleFilters, roleFilters) ||
			userIDPrefix != "" {
			return listMembersPageToken{}, false, types.NewInvalidArgument("page_token is invalid")
		}
		return decoded, true, nil
	}
	if decoded.Version == 4 {
		if userIDPrefix != "" ||
			decoded.UserID == "" ||
			decoded.Sort != sort ||
			decoded.RoleFilter != string(roleFilter) ||
			!sameStringSlice(decoded.RoleFilters, roleFilters) {
			return listMembersPageToken{}, false, types.NewInvalidArgument("page_token is invalid")
		}
		if sort == types.ConversationMemberListSortRoleUserIDAsc && memberRoleSortWeight(types.MemberRole(decoded.Role)) == 0 {
			return listMembersPageToken{}, false, types.NewInvalidArgument("page_token is invalid")
		}
		return decoded, true, nil
	}
	if decoded.Version != 5 ||
		decoded.UserID == "" ||
		decoded.Sort != sort ||
		decoded.RoleFilter != string(roleFilter) ||
		!sameStringSlice(decoded.RoleFilters, roleFilters) ||
		decoded.UserIDPrefix != userIDPrefix {
		return listMembersPageToken{}, false, types.NewInvalidArgument("page_token is invalid")
	}
	if sort == types.ConversationMemberListSortRoleUserIDAsc && memberRoleSortWeight(types.MemberRole(decoded.Role)) == 0 {
		return listMembersPageToken{}, false, types.NewInvalidArgument("page_token is invalid")
	}
	return decoded, true, nil
}

func encodeListMembersPageToken(member types.ConversationMember, roleFilter types.MemberRole, roleFilters []string, sort string, userIDPrefix string) (string, error) {
	payload, err := json.Marshal(listMembersPageToken{
		Version:      5,
		UserID:       string(member.UserID),
		Role:         string(member.Role),
		Sort:         sort,
		RoleFilter:   string(roleFilter),
		RoleFilters:  roleFilters,
		UserIDPrefix: userIDPrefix,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func memberRolesToStrings(roles []types.MemberRole) []string {
	values := make([]string, 0, len(roles))
	for _, role := range roles {
		values = append(values, string(role))
	}
	return values
}

func memberRoleSortWeight(role types.MemberRole) int {
	switch role {
	case types.MemberRoleOwner:
		return 1
	case types.MemberRoleAdmin:
		return 2
	case types.MemberRoleMember:
		return 3
	default:
		return 0
	}
}

func sameStringSlice(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
