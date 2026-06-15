package types

import "time"

const (
	DefaultConversationMembersPageSize = 100
	MaxConversationMembersPageSize     = 500
)

type ListConversationMembersCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	PageSize       int
	PageToken      string
	RoleFilter     MemberRole
}

func (c ListConversationMembersCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("auth_context.tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("auth_context.user_id is required")
	}
	if c.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if c.PageSize < 0 {
		return NewInvalidArgument("page_size is invalid")
	}
	if c.PageSize > MaxConversationMembersPageSize {
		return NewInvalidArgument("page_size exceeds max")
	}
	if c.RoleFilter != "" && !isValidListMemberRoleFilter(c.RoleFilter) {
		return NewInvalidArgument("role_filter is invalid")
	}
	return nil
}

func (c ListConversationMembersCommand) EffectivePageSize() int {
	if c.PageSize == 0 {
		return DefaultConversationMembersPageSize
	}
	return c.PageSize
}

func isValidListMemberRoleFilter(role MemberRole) bool {
	switch role {
	case MemberRoleOwner, MemberRoleAdmin, MemberRoleMember:
		return true
	default:
		return false
	}
}

type ConversationMember struct {
	UserID            UserID
	Role              MemberRole
	Status            MemberStatus
	JoinSeq           int64
	LeaveSeq          int64
	MemberVersion     int64
	PermissionVersion int64
	UpdatedAt         time.Time
}

type ListConversationMembersResult struct {
	TenantID          TenantID
	ConversationID    ConversationID
	MemberVersion     int64
	PermissionVersion int64
	Members           []ConversationMember
	NextPageToken     string
}
