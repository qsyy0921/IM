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

func (context AuthContext) Normalized() AuthContext {
	context.TenantID = TenantID(strings.TrimSpace(string(context.TenantID)))
	context.UserID = strings.TrimSpace(context.UserID)
	context.ServiceName = strings.TrimSpace(context.ServiceName)
	context.InstanceRef = strings.TrimSpace(context.InstanceRef)
	context.TraceID = strings.TrimSpace(context.TraceID)
	context.RequestID = strings.TrimSpace(context.RequestID)
	return context
}

func (context AuthContext) Validate() error {
	context = context.Normalized()
	if context.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if context.UserID == "" && context.ServiceName == "" {
		return NewInvalidArgument("user_id or service_name is required")
	}
	return nil
}
