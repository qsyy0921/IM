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

func (auth AuthContext) IsValid() bool {
	return auth.TenantID != "" && auth.UserID != "" && auth.DeviceID != ""
}
