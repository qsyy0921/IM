package types

import "strings"

type TenantID string

type AuthContext struct {
	TenantID    TenantID
	UserID      string
	ServiceName string
	InstanceRef string
	TraceID     string
	RequestID   string
}

func (auth AuthContext) ValidateTenant() error {
	if strings.TrimSpace(string(auth.TenantID)) == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	return nil
}

func (auth AuthContext) IsService() bool {
	return strings.TrimSpace(auth.ServiceName) != ""
}
