package types

type TenantID string

type AuthContext struct {
	TenantID    TenantID
	UserID      string
	ServiceName string
	InstanceRef string
	TraceID     string
	RequestID   string
}

func (auth AuthContext) Valid() bool {
	return auth.TenantID != ""
}
