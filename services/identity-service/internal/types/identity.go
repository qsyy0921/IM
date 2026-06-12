package types

type TenantID string
type UserID string
type DeviceID string
type SessionID string
type RefreshTokenID string

type DeviceStatus string
type SessionStatus string
type UserStatus string

const (
	DeviceStatusActive  DeviceStatus = "ACTIVE"
	DeviceStatusRevoked DeviceStatus = "REVOKED"

	SessionStatusActive  SessionStatus = "ACTIVE"
	SessionStatusRevoked SessionStatus = "REVOKED"

	UserStatusActive UserStatus = "ACTIVE"
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

type RegisterUserCommand struct {
	TenantID  TenantID
	UserID    UserID
	Password  string
	TraceID   string
	RequestID string
}

type RegisterUserResult struct {
	TenantID        TenantID
	UserID          UserID
	Status          UserStatus
	CreatedAtUnixMS int64
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

type LoginCommand struct {
	TenantID          TenantID
	UserID            UserID
	Password          string
	DeviceID          DeviceID
	Audience          string
	GatewayTTLSeconds int64
	RefreshTTLSeconds int64
	TraceID           string
	RequestID         string
}

type LoginResult struct {
	TenantID               TenantID
	UserID                 UserID
	DeviceID               DeviceID
	SessionID              SessionID
	Audience               string
	TokenType              string
	GatewayToken           string
	RefreshToken           string
	GatewayExpiresAtUnixMS int64
	RefreshExpiresAtUnixMS int64
	IssuedAtUnixMS         int64
}

type RefreshGatewayTokenCommand struct {
	TenantID          TenantID
	UserID            UserID
	DeviceID          DeviceID
	RefreshToken      string
	Audience          string
	GatewayTTLSeconds int64
	RefreshTTLSeconds int64
	TraceID           string
	RequestID         string
}

type RefreshGatewayTokenResult struct {
	TenantID               TenantID
	UserID                 UserID
	DeviceID               DeviceID
	SessionID              SessionID
	Audience               string
	TokenType              string
	GatewayToken           string
	RefreshToken           string
	GatewayExpiresAtUnixMS int64
	RefreshExpiresAtUnixMS int64
	IssuedAtUnixMS         int64
}

type UserCredential struct {
	TenantID     TenantID
	UserID       UserID
	Status       string
	PasswordHash string
}

type RefreshTokenRecord struct {
	TokenID   RefreshTokenID
	TokenHash string
}

type ParsedRefreshToken struct {
	TokenID RefreshTokenID
	Secret  string
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
	Issuer    string
	TraceID   string
	IssuedAt  int64
	ExpiresAt int64
}
