package types

import "strings"

type TenantID string
type UserID string
type ConversationID string

type AuthContext struct {
	TenantID  TenantID
	UserID    UserID
	DeviceID  string
	SessionID string
	TraceID   string
	RequestID string
}

func (auth AuthContext) Validate() error {
	if strings.TrimSpace(string(auth.TenantID)) == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if strings.TrimSpace(string(auth.UserID)) == "" {
		return NewInvalidArgument("user_id is required")
	}
	return nil
}
