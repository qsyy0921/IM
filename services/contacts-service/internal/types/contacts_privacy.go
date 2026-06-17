package types

import "strings"

type ContactPrivacySettings struct {
	AllowContactRequests       bool
	AllowSearchContactRequests bool
	AllowProfileVisibility     bool
	ProfileVisibilityFields    []ContactProfileVisibilityField
	Version                    int64
	UpdatedAtUnixMS            int64
	PolicySource               ContactPrivacyPolicySource
}

type GetContactPrivacyCommand struct {
	AuthContext AuthContext
}

func (c GetContactPrivacyCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	return nil
}

type GetContactPrivacyResult struct {
	TenantID TenantID
	UserID   UserID
	Settings ContactPrivacySettings
}

type SetContactPrivacyCommand struct {
	AuthContext                   AuthContext
	AllowContactRequests          bool
	AllowSearchContactRequests    *bool
	AllowProfileVisibility        *bool
	UpdateProfileVisibilityFields bool
	ProfileVisibilityFields       []ContactProfileVisibilityField
	IdempotencyKey                string
}

func (c SetContactPrivacyCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if strings.TrimSpace(c.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if c.UpdateProfileVisibilityFields {
		if _, err := NormalizeContactProfileVisibilityFields(c.ProfileVisibilityFields); err != nil {
			return err
		}
	}
	return nil
}

type SetContactPrivacyResult struct {
	TenantID         TenantID
	UserID           UserID
	Settings         ContactPrivacySettings
	IdempotentReplay bool
}

type SetContactPrivacyExceptionCommand struct {
	AuthContext    AuthContext
	OtherUserID    UserID
	Decision       ContactPrivacyExceptionDecision
	IdempotencyKey string
}

func (c SetContactPrivacyExceptionCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if c.OtherUserID == "" {
		return NewInvalidArgument("other_user_id is required")
	}
	if c.AuthContext.UserID == c.OtherUserID {
		return NewInvalidArgument("other_user_id must differ from user_id")
	}
	if strings.TrimSpace(c.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if NormalizeContactPrivacyExceptionDecision(c.Decision) == "" {
		return NewInvalidArgument("decision is invalid")
	}
	return nil
}

func NormalizeContactPrivacyExceptionDecision(value ContactPrivacyExceptionDecision) ContactPrivacyExceptionDecision {
	switch ContactPrivacyExceptionDecision(strings.ToUpper(strings.TrimSpace(string(value)))) {
	case ContactPrivacyExceptionDecisionAllow:
		return ContactPrivacyExceptionDecisionAllow
	case ContactPrivacyExceptionDecisionDeny:
		return ContactPrivacyExceptionDecisionDeny
	default:
		return ""
	}
}

type SetContactPrivacyExceptionResult struct {
	TenantID         TenantID
	OwnerUserID      UserID
	OtherUserID      UserID
	Decision         ContactPrivacyExceptionDecision
	Version          int64
	IdempotentReplay bool
}

type ContactPrivacyExceptionItem struct {
	OtherUserID     UserID
	Decision        ContactPrivacyExceptionDecision
	Version         int64
	UpdatedAtUnixMS int64
}

type ListContactPrivacyExceptionsCommand struct {
	AuthContext AuthContext
	PageSize    int
	PageToken   string
}

func (c ListContactPrivacyExceptionsCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if c.PageSize < 0 {
		return NewInvalidArgument("page_size must be non-negative")
	}
	return nil
}

type ListContactPrivacyExceptionsResult struct {
	TenantID      TenantID
	OwnerUserID   UserID
	Exceptions    []ContactPrivacyExceptionItem
	NextPageToken string
}

type DeleteContactPrivacyExceptionCommand struct {
	AuthContext    AuthContext
	OtherUserID    UserID
	IdempotencyKey string
}

func (c DeleteContactPrivacyExceptionCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if c.OtherUserID == "" {
		return NewInvalidArgument("other_user_id is required")
	}
	if c.AuthContext.UserID == c.OtherUserID {
		return NewInvalidArgument("other_user_id must differ from user_id")
	}
	if strings.TrimSpace(c.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	return nil
}

type DeleteContactPrivacyExceptionResult struct {
	TenantID         TenantID
	OwnerUserID      UserID
	OtherUserID      UserID
	Deleted          bool
	IdempotentReplay bool
}

type GetTenantContactPrivacyDefaultCommand struct {
	TenantID TenantID
}

func (c GetTenantContactPrivacyDefaultCommand) Validate() error {
	if c.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	return nil
}

type GetTenantContactPrivacyDefaultResult struct {
	TenantID TenantID
	Settings ContactPrivacySettings
}

type SetTenantContactPrivacyDefaultCommand struct {
	TenantID                      TenantID
	AllowContactRequests          bool
	AllowSearchContactRequests    *bool
	AllowProfileVisibility        *bool
	UpdateProfileVisibilityFields bool
	ProfileVisibilityFields       []ContactProfileVisibilityField
}

func (c SetTenantContactPrivacyDefaultCommand) Validate() error {
	if c.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.UpdateProfileVisibilityFields {
		if _, err := NormalizeContactProfileVisibilityFields(c.ProfileVisibilityFields); err != nil {
			return err
		}
	}
	return nil
}

func DefaultContactProfileVisibilityFields() []ContactProfileVisibilityField {
	return []ContactProfileVisibilityField{
		ContactProfileVisibilityFieldDisplayName,
		ContactProfileVisibilityFieldAvatar,
		ContactProfileVisibilityFieldOrganization,
		ContactProfileVisibilityFieldTitle,
	}
}

func NormalizeContactProfileVisibilityFields(fields []ContactProfileVisibilityField) ([]ContactProfileVisibilityField, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	seen := make(map[ContactProfileVisibilityField]struct{}, len(fields))
	normalized := make([]ContactProfileVisibilityField, 0, len(fields))
	for _, field := range fields {
		value := ContactProfileVisibilityField(strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(string(field), "-", "_"))))
		if value == "" {
			return nil, NewInvalidArgument("profile_visibility_fields contains unsupported field")
		}
		switch value {
		case ContactProfileVisibilityFieldDisplayName,
			ContactProfileVisibilityFieldAvatar,
			ContactProfileVisibilityFieldOrganization,
			ContactProfileVisibilityFieldTitle,
			ContactProfileVisibilityFieldStatusMessage:
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			normalized = append(normalized, value)
		default:
			return nil, NewInvalidArgument("profile_visibility_fields contains unsupported field")
		}
	}
	return normalized, nil
}

func ContactProfileVisibilityFieldsToStrings(fields []ContactProfileVisibilityField) []string {
	if len(fields) == 0 {
		return []string{}
	}
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		values = append(values, string(field))
	}
	return values
}

func ContactProfileVisibilityFieldsFromStrings(values []string) []ContactProfileVisibilityField {
	if len(values) == 0 {
		return nil
	}
	fields := make([]ContactProfileVisibilityField, 0, len(values))
	for _, value := range values {
		fields = append(fields, ContactProfileVisibilityField(value))
	}
	normalized, err := NormalizeContactProfileVisibilityFields(fields)
	if err != nil {
		return nil
	}
	return normalized
}

type SetTenantContactPrivacyDefaultResult struct {
	TenantID TenantID
	Settings ContactPrivacySettings
	Changed  bool
}
