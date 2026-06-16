package types

import (
	"sort"
	"time"
)

const (
	DefaultConversationMembersPageSize = 100
	MaxConversationMembersPageSize     = 500
	MaxConversationMemberUserIDPrefix  = 128
)

type ListConversationMembersCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	PageSize       int
	PageToken      string
	RoleFilter     MemberRole
	RoleFilters    []MemberRole
	Sort           string
	UserIDPrefix   string
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
	if _, err := NormalizeListMemberRoleFilters(c.RoleFilters); err != nil {
		return err
	}
	if _, err := NormalizeConversationMemberListSort(c.Sort); err != nil {
		return err
	}
	if err := ValidateConversationMemberUserIDPrefix(c.UserIDPrefix); err != nil {
		return err
	}
	return nil
}

func (c ListConversationMembersCommand) EffectivePageSize() int {
	if c.PageSize == 0 {
		return DefaultConversationMembersPageSize
	}
	return c.PageSize
}

func ValidateConversationMemberUserIDPrefix(prefix string) error {
	if len(prefix) > MaxConversationMemberUserIDPrefix {
		return NewInvalidArgument("user_id_prefix is too long")
	}
	for _, char := range prefix {
		if char == 0 {
			return NewInvalidArgument("user_id_prefix contains unsupported characters")
		}
	}
	return nil
}

const (
	ConversationMemberListSortUserIDAsc     = "user_id_asc"
	ConversationMemberListSortRoleUserIDAsc = "role_user_id_asc"
)

func NormalizeConversationMemberListSort(sort string) (string, error) {
	if sort == "" {
		return ConversationMemberListSortUserIDAsc, nil
	}
	switch sort {
	case ConversationMemberListSortUserIDAsc, ConversationMemberListSortRoleUserIDAsc:
		return sort, nil
	default:
		return "", NewInvalidArgument("member list sort is invalid")
	}
}

func isValidListMemberRoleFilter(role MemberRole) bool {
	switch role {
	case MemberRoleOwner, MemberRoleAdmin, MemberRoleMember:
		return true
	default:
		return false
	}
}

func NormalizeListMemberRoleFilters(filters []MemberRole) ([]MemberRole, error) {
	if len(filters) == 0 {
		return []MemberRole{}, nil
	}
	seen := make(map[MemberRole]bool, len(filters))
	normalized := make([]MemberRole, 0, len(filters))
	for _, filter := range filters {
		if !isValidListMemberRoleFilter(filter) {
			return nil, NewInvalidArgument("role_filters is invalid")
		}
		if seen[filter] {
			continue
		}
		seen[filter] = true
		normalized = append(normalized, filter)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i] < normalized[j]
	})
	return normalized, nil
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
