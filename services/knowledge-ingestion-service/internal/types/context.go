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

func (auth AuthContext) ValidateService() error {
	if err := auth.ValidateTenant(); err != nil {
		return err
	}
	if strings.TrimSpace(auth.ServiceName) == "" {
		return NewPermissionDenied("service identity is required")
	}
	return nil
}
