package types

type AuthContext struct {
	TenantID  TenantID
	UserID    UserID
	DeviceID  DeviceID
	SessionID string
	TraceID   string
	RequestID string
}

type MessageAction string

const (
	MessageActionSend   MessageAction = "SEND"
	MessageActionEdit   MessageAction = "EDIT"
	MessageActionRevoke MessageAction = "REVOKE"
	MessageActionDelete MessageAction = "DELETE"
)

type CheckMessageActionCommand struct {
	AuthContext                   AuthContext
	ConversationID                ConversationID
	Action                        MessageAction
	MessageID                     MessageID
	DirectPeerUserID              UserID
	MessageSenderUserID           UserID
	ConversationPermissionVersion int64
}

func (c CheckMessageActionCommand) Validate() error {
	if c.AuthContext.TenantID == "" || c.AuthContext.UserID == "" || c.AuthContext.DeviceID == "" {
		return NewInvalidArgument("auth context is required")
	}
	if c.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	switch c.Action {
	case MessageActionSend:
	case MessageActionEdit, MessageActionRevoke, MessageActionDelete:
		if c.MessageID == "" {
			return NewInvalidArgument("message_id is required")
		}
	default:
		return NewInvalidArgument("message action is required")
	}
	if c.DirectPeerUserID != "" && c.DirectPeerUserID == c.AuthContext.UserID {
		return NewInvalidArgument("direct_peer_user_id must not equal auth user")
	}
	return nil
}

type MessageActionDecision struct {
	TenantID          TenantID
	UserID            UserID
	ConversationID    ConversationID
	MessageID         MessageID
	Action            MessageAction
	Allowed           bool
	PermissionVersion int64
	Classification    string
	Reason            string
	OwnershipOverride bool
}
