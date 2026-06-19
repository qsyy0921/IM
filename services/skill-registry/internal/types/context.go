package types

type TenantID string
type UserID string

type AuthContext struct {
	TenantID  TenantID
	UserID    UserID
	DeviceID  string
	SessionID string
	TraceID   string
	RequestID string
}

func (auth AuthContext) Validate() error {
	if auth.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if auth.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if auth.DeviceID == "" {
		return NewInvalidArgument("device_id is required")
	}
	return nil
}
