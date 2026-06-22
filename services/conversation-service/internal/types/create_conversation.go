package types

type CreateConversationCommand struct {
	AuthContext      AuthContext
	ConversationID   ConversationID
	ConversationType ConversationType
	DirectPeerUserID UserID
	IdempotencyKey   string
}

func (c CreateConversationCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("auth_context.tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("auth_context.user_id is required")
	}
	if c.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if c.IdempotencyKey == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	switch c.ConversationType {
	case ConversationTypeGroup:
		if c.DirectPeerUserID != "" {
			return NewInvalidArgument("direct_peer_user_id is only allowed for direct conversation")
		}
	case ConversationTypeDirect:
		if c.DirectPeerUserID == "" {
			return NewInvalidArgument("direct_peer_user_id is required")
		}
		if c.DirectPeerUserID == c.AuthContext.UserID {
			return NewInvalidArgument("direct_peer_user_id must differ from current user")
		}
	default:
		return NewInvalidArgument("conversation_type is not supported")
	}
	return nil
}

type CreateConversationResult struct {
	TenantID          TenantID
	ConversationID    ConversationID
	ConversationType  ConversationType
	DirectPeerUserID  UserID
	BoundarySeq       int64
	MemberVersion     int64
	PermissionVersion int64
	IdempotentReplay  bool
}
