package types

type AuthContext struct {
	TenantID  TenantID
	UserID    UserID
	DeviceID  string
	SessionID string
	TraceID   string
	RequestID string
}

type CreateMemberChangeCommand struct {
	AuthContext           AuthContext
	ConversationID        ConversationID
	TargetUserID          UserID
	ChangeType            MemberChangeType
	TargetRole            MemberRole
	ExpectedMemberVersion int64
	IdempotencyKey        string
	ConflictPolicy        MemberChangeConflictPolicy
	Reason                string
}

func (c CreateMemberChangeCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("auth_context.tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("auth_context.user_id is required")
	}
	if c.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if c.TargetUserID == "" {
		return NewInvalidArgument("target_user_id is required")
	}
	if c.IdempotencyKey == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if !isValidMemberChangeType(c.ChangeType) {
		return NewInvalidArgument("change_type is invalid")
	}
	if c.ChangeType == MemberChangeTypeJoin || c.ChangeType == MemberChangeTypeRoleChanged {
		if !isValidMemberRole(c.TargetRole) {
			return NewInvalidArgument("target_role is required")
		}
	}
	if c.ConflictPolicy == "" {
		return NewInvalidArgument("conflict_policy is required")
	}
	if !isValidConflictPolicy(c.ConflictPolicy) {
		return NewInvalidArgument("conflict_policy is invalid")
	}
	return nil
}

type MemberChangeResult struct {
	ChangeID          ChangeID
	TenantID          TenantID
	ConversationID    ConversationID
	TargetUserID      UserID
	OperatorUserID    UserID
	ChangeType        MemberChangeType
	Status            MemberChangeStatus
	BoundarySeq       int64
	MemberVersion     int64
	PermissionVersion int64
	IdempotentReplay  bool
}

type TransferConversationOwnerCommand struct {
	AuthContext           AuthContext
	ConversationID        ConversationID
	NewOwnerUserID        UserID
	ExpectedMemberVersion int64
	IdempotencyKey        string
	Reason                string
}

func (c TransferConversationOwnerCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("auth_context.tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("auth_context.user_id is required")
	}
	if c.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if c.NewOwnerUserID == "" {
		return NewInvalidArgument("new_owner_user_id is required")
	}
	if c.NewOwnerUserID == c.AuthContext.UserID {
		return NewInvalidArgument("new_owner_user_id must differ from current owner")
	}
	if c.IdempotencyKey == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	return nil
}

type TransferConversationOwnerResult struct {
	ChangeID            ChangeID
	TenantID            TenantID
	ConversationID      ConversationID
	PreviousOwnerUserID UserID
	NewOwnerUserID      UserID
	Status              MemberChangeStatus
	BoundarySeq         int64
	MemberVersion       int64
	PermissionVersion   int64
	IdempotentReplay    bool
}

type GetMemberChangeCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	ChangeID       ChangeID
}

func (c GetMemberChangeCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("auth_context.tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("auth_context.user_id is required")
	}
	if c.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if c.ChangeID == "" {
		return NewInvalidArgument("change_id is required")
	}
	return nil
}

type MemberChangeDetail struct {
	ChangeID          ChangeID
	TenantID          TenantID
	ConversationID    ConversationID
	TargetUserID      UserID
	OperatorUserID    UserID
	ChangeType        MemberChangeType
	Status            MemberChangeStatus
	BoundarySeq       int64
	MemberVersion     int64
	PermissionVersion int64
	OldRole           MemberRole
	NewRole           MemberRole
	Reason            string
	LastError         string
}

type MemberChangePublishProgressStats struct {
	Advanced int
}

func isValidMemberChangeType(value MemberChangeType) bool {
	switch value {
	case MemberChangeTypeJoin, MemberChangeTypeLeave, MemberChangeTypeRemove, MemberChangeTypeRoleChanged:
		return true
	default:
		return false
	}
}

func isValidMemberRole(value MemberRole) bool {
	switch value {
	case MemberRoleOwner, MemberRoleAdmin, MemberRoleMember:
		return true
	default:
		return false
	}
}

func isValidConflictPolicy(value MemberChangeConflictPolicy) bool {
	switch value {
	case MemberChangeConflictPolicyReject:
		return true
	default:
		return false
	}
}
