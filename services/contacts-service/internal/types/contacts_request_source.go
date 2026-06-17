package types

import (
	"strings"
	"unicode"
)

type SendContactRequestCommand struct {
	AuthContext    AuthContext
	TargetUserID   UserID
	IdempotencyKey string
	Message        string
	SourceType     ContactRequestSourceType
	SourceRef      string
}

func (c SendContactRequestCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if c.TargetUserID == "" {
		return NewInvalidArgument("target_user_id is required")
	}
	if c.AuthContext.UserID == c.TargetUserID {
		return NewInvalidArgument("target_user_id must differ from user_id")
	}
	if strings.TrimSpace(c.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if c.NormalizedSourceType() == "" {
		return NewInvalidArgument("source_type is invalid")
	}
	if len(c.NormalizedSourceRef()) > maxContactSourceRefLength {
		return NewInvalidArgument("source_ref is too long")
	}
	if containsSensitiveContactSourceRef(c.NormalizedSourceRef()) {
		return NewInvalidArgument("source_ref must not contain sensitive data")
	}
	return nil
}

func (c SendContactRequestCommand) NormalizedSourceType() ContactRequestSourceType {
	return NormalizeContactRequestSourceType(c.SourceType)
}

func (c SendContactRequestCommand) NormalizedSourceRef() string {
	return strings.TrimSpace(c.SourceRef)
}

func containsSensitiveContactSourceRef(sourceRef string) bool {
	if sourceRef == "" {
		return false
	}
	if strings.Contains(sourceRef, "@") {
		return true
	}
	if isPhoneLikeSourceRef(sourceRef) {
		return true
	}
	for _, part := range strings.FieldsFunc(strings.ToLower(sourceRef), sourceRefSeparator) {
		switch part {
		case "auth", "authorization", "bearer", "cookie", "credential", "credentials",
			"email", "jwt", "mobile", "password", "passwd", "phone", "secret", "session", "token":
			return true
		}
	}
	for _, r := range sourceRef {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func sourceRefSeparator(r rune) bool {
	switch r {
	case ':', '=', '&', '?', '/', '#', ';', ',', '|', ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func isPhoneLikeSourceRef(sourceRef string) bool {
	digits := 0
	for _, r := range sourceRef {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '+' || r == '-' || r == ' ' || r == '(' || r == ')':
			continue
		default:
			return false
		}
	}
	return digits >= 10 && digits <= 15
}

func NormalizeContactRequestSourceType(value ContactRequestSourceType) ContactRequestSourceType {
	if value == "" {
		return ContactRequestSourceTypeDirect
	}
	switch value {
	case ContactRequestSourceTypeDirect,
		ContactRequestSourceTypeSearch,
		ContactRequestSourceTypeGroup,
		ContactRequestSourceTypeInviteLink,
		ContactRequestSourceTypeQRCode,
		ContactRequestSourceTypeImport:
		return value
	default:
		return ""
	}
}

func NormalizeContactRequestRiskLevel(value ContactRequestRiskLevel) ContactRequestRiskLevel {
	if value == "" {
		return ContactRequestRiskLevelLow
	}
	switch value {
	case ContactRequestRiskLevelLow,
		ContactRequestRiskLevelMedium,
		ContactRequestRiskLevelHigh:
		return value
	default:
		return ""
	}
}

type SendContactRequestResult struct {
	RequestID        string
	TenantID         TenantID
	SenderUserID     UserID
	ReceiverUserID   UserID
	Status           ContactRequestStatus
	IdempotentReplay bool
	SourceType       ContactRequestSourceType
	SourceRef        string
	RiskLevel        ContactRequestRiskLevel
	ReviewRequired   bool
}
