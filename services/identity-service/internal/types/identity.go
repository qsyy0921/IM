package types

import "time"

type TenantID string
type UserID string
type DeviceID string
type SessionID string
type RefreshTokenID string

type DeviceStatus string
type SessionStatus string
type UserStatus string
type VerificationChannel string
type ChallengeType string
type ChallengeID string
type MFAFactorID string
type MFAFactorType string
type MFAFactorStatus string

const (
	DeviceStatusActive  DeviceStatus = "ACTIVE"
	DeviceStatusRevoked DeviceStatus = "REVOKED"

	SessionStatusActive  SessionStatus = "ACTIVE"
	SessionStatusRevoked SessionStatus = "REVOKED"

	UserStatusActive UserStatus = "ACTIVE"

	VerificationChannelEmail VerificationChannel = "EMAIL"
	VerificationChannelPhone VerificationChannel = "PHONE"

	ChallengeTypeEmailVerification ChallengeType = "EMAIL_VERIFICATION"
	ChallengeTypePhoneVerification ChallengeType = "PHONE_VERIFICATION"
	ChallengeTypePasswordReset     ChallengeType = "PASSWORD_RESET"

	MFAFactorTypeTOTP MFAFactorType = "TOTP"

	MFAFactorStatusPending  MFAFactorStatus = "PENDING"
	MFAFactorStatusActive   MFAFactorStatus = "ACTIVE"
	MFAFactorStatusDisabled MFAFactorStatus = "DISABLED"
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
	MFAFactorID       MFAFactorID
	MFACode           string
	MFARecoveryCode   string
	// VerifiedMFAFactorID is set only by LoginUseCase after TOTP verification.
	// API adapters and repository tests must not use it as caller-supplied proof.
	VerifiedMFAFactorID MFAFactorID
	// UsedMFARecoveryCode is set only by LoginUseCase after recovery-code verification.
	// API adapters and repository tests must not use it as caller-supplied proof.
	UsedMFARecoveryCode MFARecoveryCodeRecord
	TraceID             string
	RequestID           string
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
	MFAFactorID       MFAFactorID
	MFACode           string
	MFARecoveryCode   string
	// VerifiedMFAFactorID is set only by RefreshGatewayTokenUseCase after TOTP verification.
	// API adapters and repository tests must not use it as caller-supplied proof.
	VerifiedMFAFactorID MFAFactorID
	// UsedMFARecoveryCode is set only by RefreshGatewayTokenUseCase after recovery-code verification.
	// API adapters and repository tests must not use it as caller-supplied proof.
	UsedMFARecoveryCode MFARecoveryCodeRecord
	TraceID             string
	RequestID           string
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

type RequestVerificationChallengeCommand struct {
	TenantID    TenantID
	UserID      UserID
	Channel     VerificationChannel
	Destination string
	TTLSeconds  int64
	Password    string
	TraceID     string
	RequestID   string
}

type RequestVerificationChallengeResult struct {
	TenantID          TenantID
	UserID            UserID
	ChallengeID       ChallengeID
	Channel           VerificationChannel
	Destination       string
	ExpiresAtUnixMS   int64
	DevChallengeToken string
}

type ConfirmVerificationChallengeCommand struct {
	TenantID       TenantID
	UserID         UserID
	ChallengeID    ChallengeID
	ChallengeToken string
	TraceID        string
	RequestID      string
}

type ConfirmVerificationChallengeResult struct {
	TenantID         TenantID
	UserID           UserID
	Channel          VerificationChannel
	Destination      string
	VerifiedAtUnixMS int64
}

type RequestPasswordResetCommand struct {
	TenantID    TenantID
	UserID      UserID
	Channel     VerificationChannel
	Destination string
	TTLSeconds  int64
	TraceID     string
	RequestID   string
}

type RequestPasswordResetResult struct {
	TenantID          TenantID
	UserID            UserID
	ChallengeID       ChallengeID
	Channel           VerificationChannel
	Destination       string
	ExpiresAtUnixMS   int64
	DevChallengeToken string
}

type ChallengeNotification struct {
	TenantID        TenantID            `json:"tenant_id"`
	UserID          UserID              `json:"user_id"`
	ChallengeID     ChallengeID         `json:"challenge_id"`
	Type            ChallengeType       `json:"challenge_type"`
	Channel         VerificationChannel `json:"channel"`
	Destination     string              `json:"destination"`
	Token           string              `json:"token"`
	ExpiresAtUnixMS int64               `json:"expires_at_unix_ms"`
	TraceID         string              `json:"trace_id,omitempty"`
	RequestID       string              `json:"request_id,omitempty"`
}

type ConfirmPasswordResetCommand struct {
	TenantID       TenantID
	UserID         UserID
	ChallengeID    ChallengeID
	ChallengeToken string
	NewPassword    string
	TraceID        string
	RequestID      string
}

type ConfirmPasswordResetResult struct {
	TenantID      TenantID
	UserID        UserID
	ResetAtUnixMS int64
}

type BeginMFAEnrollmentCommand struct {
	TenantID    TenantID
	UserID      UserID
	FactorType  MFAFactorType
	Password    string
	DisplayName string
	Issuer      string
	TraceID     string
	RequestID   string
}

type BeginMFAEnrollmentResult struct {
	TenantID        TenantID
	UserID          UserID
	FactorID        MFAFactorID
	FactorType      MFAFactorType
	Status          MFAFactorStatus
	Secret          string
	OTPAuthURI      string
	CreatedAtUnixMS int64
}

type ConfirmMFAEnrollmentCommand struct {
	TenantID  TenantID
	UserID    UserID
	FactorID  MFAFactorID
	Code      string
	TraceID   string
	RequestID string
}

type ConfirmMFAEnrollmentResult struct {
	TenantID         TenantID
	UserID           UserID
	FactorID         MFAFactorID
	Status           MFAFactorStatus
	VerifiedAtUnixMS int64
	RecoveryCodes    []string
}

type DisableMFAFactorCommand struct {
	TenantID  TenantID
	UserID    UserID
	FactorID  MFAFactorID
	Password  string
	TraceID   string
	RequestID string
}

type DisableMFAFactorResult struct {
	TenantID         TenantID
	UserID           UserID
	FactorID         MFAFactorID
	Status           MFAFactorStatus
	DisabledAtUnixMS int64
}

type RegenerateMFARecoveryCodesCommand struct {
	TenantID  TenantID
	UserID    UserID
	FactorID  MFAFactorID
	Password  string
	Code      string
	TraceID   string
	RequestID string
}

type RegenerateMFARecoveryCodesResult struct {
	TenantID          TenantID
	UserID            UserID
	FactorID          MFAFactorID
	RecoveryCodes     []string
	GeneratedAtUnixMS int64
}

type RevokeMFARecoveryCodesCommand struct {
	TenantID  TenantID
	UserID    UserID
	Password  string
	TraceID   string
	RequestID string
}

type RevokeMFARecoveryCodesResult struct {
	TenantID        TenantID
	UserID          UserID
	RevokedCount    int
	RevokedAtUnixMS int64
}

type ChallengeRecord struct {
	ChallengeID ChallengeID
	TokenHash   string
}

type EncryptedMFASecret struct {
	Ciphertext string
	Nonce      string
	KeyVersion string
}

type MFAFactorSecret struct {
	TenantID         TenantID
	UserID           UserID
	FactorID         MFAFactorID
	Type             MFAFactorType
	Status           MFAFactorStatus
	Secret           EncryptedMFASecret
	LoginFailedCount int
	LoginLockedUntil time.Time
}

type MFARecoveryCodeRecord struct {
	CodeID   string
	CodeHash string
}

type MFARecoveryCode struct {
	CodeID   string
	Code     string
	CodeHash string
}

type IdentityChallenge struct {
	TenantID    TenantID
	UserID      UserID
	ChallengeID ChallengeID
	Type        ChallengeType
	Channel     VerificationChannel
	Destination string
}

type UserCredential struct {
	TenantID               TenantID
	UserID                 UserID
	Status                 string
	PasswordHash           string
	FailedLoginCount       int
	LockedUntil            time.Time
	MFARecoveryFailedCount int
	MFARecoveryLockedUntil time.Time
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
