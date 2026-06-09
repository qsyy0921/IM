package types

type GetSendContextCommand struct {
	TenantID       TenantID
	ConversationID ConversationID
	UserID         UserID
	TraceID        string
}

func (c GetSendContextCommand) Validate() error {
	if c.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if c.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	return nil
}
