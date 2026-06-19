package types

import "strings"

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
	return strings.TrimSpace(string(auth.TenantID)) != "" &&
		strings.TrimSpace(string(auth.UserID)) != "" &&
		strings.TrimSpace(auth.DeviceID) != ""
}
