package types

import "errors"

type GetSendContextCommand struct {
	TenantID       TenantID
	ConversationID ConversationID
	UserID         UserID
	TraceID        string
}

func (c GetSendContextCommand) Validate() error {
	if c.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	if c.ConversationID == "" {
		return errors.New("conversation_id is required")
	}
	if c.UserID == "" {
		return errors.New("user_id is required")
	}
	return nil
}
