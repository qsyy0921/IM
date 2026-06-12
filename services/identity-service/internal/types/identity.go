package types

type TenantID string
type UserID string
type DeviceID string
type SessionID string

type DeviceStatus string
type SessionStatus string

const (
	DeviceStatusActive  DeviceStatus = "ACTIVE"
	DeviceStatusRevoked DeviceStatus = "REVOKED"

	SessionStatusActive  SessionStatus = "ACTIVE"
	SessionStatusRevoked SessionStatus = "REVOKED"
)

type AdminContext struct {
	TenantID       TenantID
	OperatorUserID UserID
	TraceID        string
	RequestID      string
}

func (ctx AdminContext) Validate() error {
	if ctx.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if ctx.OperatorUserID == "" {
		return NewInvalidArgument("operator_user_id is required")
	}
	return nil
}

type IssueGatewayTokenCommand struct {
	TenantID   TenantID
	UserID     UserID
	DeviceID   DeviceID
	SessionID  SessionID
	Audience   string
	TTLSeconds int64
	TraceID    string
	RequestID  string
}

type IssueGatewayTokenResult struct {
	TenantID        TenantID
	UserID          UserID
	DeviceID        DeviceID
	SessionID       SessionID
	Audience        string
	GatewayToken    string
	IssuedAtUnixMS  int64
	ExpiresAtUnixMS int64
}

type RevokeDeviceCommand struct {
	AdminContext AdminContext
	UserID       UserID
	DeviceID     DeviceID
	Reason       string
}

type RevokeDeviceResult struct {
	TenantID        TenantID
	UserID          UserID
	DeviceID        DeviceID
	Status          DeviceStatus
	RevokedAtUnixMS int64
}

type RevokeSessionCommand struct {
	AdminContext AdminContext
	UserID       UserID
	DeviceID     DeviceID
	SessionID    SessionID
	Reason       string
}

type RevokeSessionResult struct {
	TenantID        TenantID
	UserID          UserID
	DeviceID        DeviceID
	SessionID       SessionID
	Status          SessionStatus
	RevokedAtUnixMS int64
}

type GetDeviceStateCommand struct {
	AdminContext AdminContext
	UserID       UserID
	DeviceID     DeviceID
}

type GetDeviceStateResult struct {
	TenantID        TenantID
	UserID          UserID
	DeviceID        DeviceID
	Status          DeviceStatus
	CreatedAtUnixMS int64
	UpdatedAtUnixMS int64
	RevokedAtUnixMS int64
}

type TokenClaims struct {
	TenantID  TenantID
	UserID    UserID
	DeviceID  DeviceID
	SessionID SessionID
	Audience  string
	TraceID   string
	ExpiresAt int64
}
