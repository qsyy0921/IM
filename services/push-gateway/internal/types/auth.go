package types

type AuthContext struct {
	TenantID  string
	UserID    string
	DeviceID  string
	SessionID string
	TraceID   string
	RequestID string
}

func (ctx AuthContext) Validate() error {
	if ctx.TenantID == "" {
		return NewInvalidFrame("tenant_id is required")
	}
	if ctx.UserID == "" {
		return NewInvalidFrame("user_id is required")
	}
	if ctx.DeviceID == "" {
		return NewInvalidFrame("device_id is required")
	}
	return nil
}
