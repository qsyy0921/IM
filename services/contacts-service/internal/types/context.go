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
