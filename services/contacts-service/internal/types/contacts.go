package types

import (
	"strings"
	"unicode"
)

const (
	maxContactRemarkLength       = 128
	maxContactGroupNameLength    = 64
	maxContactReasonLength       = 512
	maxContactReviewReasonLength = 512
	maxContactSearchQueryLength  = 128
	maxContactSourceRefLength    = 256
)

type ContactRequestStatus string
type ContactEdgeStatus string
type ContactDecision string
type ContactRequestListDirection string
type ContactRequestSourceType string
type ContactRequestRiskLevel string
type ContactPrivacyPolicySource string
type ContactProfileVisibilityField string
type ContactPrivacyExceptionDecision string
type ContactRequestReviewDecision string

const (
	ContactRequestStatusPending        ContactRequestStatus = "PENDING"
	ContactRequestStatusReviewRequired ContactRequestStatus = "REVIEW_REQUIRED"
	ContactRequestStatusAccepted       ContactRequestStatus = "ACCEPTED"
	ContactRequestStatusDeclined       ContactRequestStatus = "DECLINED"
	ContactRequestStatusCanceled       ContactRequestStatus = "CANCELED"
	ContactRequestStatusExpired        ContactRequestStatus = "EXPIRED"

	ContactEdgeStatusActive  ContactEdgeStatus = "ACTIVE"
	ContactEdgeStatusDeleted ContactEdgeStatus = "DELETED"
	ContactEdgeStatusBlocked ContactEdgeStatus = "BLOCKED"

	ContactDecisionAccept  ContactDecision = "ACCEPT"
	ContactDecisionDecline ContactDecision = "DECLINE"

	ContactRequestListDirectionIncoming ContactRequestListDirection = "INCOMING"
	ContactRequestListDirectionOutgoing ContactRequestListDirection = "OUTGOING"

	ContactRequestSourceTypeDirect     ContactRequestSourceType = "DIRECT"
	ContactRequestSourceTypeSearch     ContactRequestSourceType = "SEARCH"
	ContactRequestSourceTypeGroup      ContactRequestSourceType = "GROUP"
	ContactRequestSourceTypeInviteLink ContactRequestSourceType = "INVITE_LINK"
	ContactRequestSourceTypeQRCode     ContactRequestSourceType = "QR_CODE"
	ContactRequestSourceTypeImport     ContactRequestSourceType = "IMPORT"

	ContactRequestRiskLevelLow    ContactRequestRiskLevel = "LOW"
	ContactRequestRiskLevelMedium ContactRequestRiskLevel = "MEDIUM"
	ContactRequestRiskLevelHigh   ContactRequestRiskLevel = "HIGH"

	ContactPrivacyPolicySourceUser          ContactPrivacyPolicySource = "USER"
	ContactPrivacyPolicySourceTenantDefault ContactPrivacyPolicySource = "TENANT_DEFAULT"
	ContactPrivacyPolicySourceSystemDefault ContactPrivacyPolicySource = "SYSTEM_DEFAULT"

	ContactProfileVisibilityFieldDisplayName   ContactProfileVisibilityField = "DISPLAY_NAME"
	ContactProfileVisibilityFieldAvatar        ContactProfileVisibilityField = "AVATAR"
	ContactProfileVisibilityFieldOrganization  ContactProfileVisibilityField = "ORGANIZATION"
	ContactProfileVisibilityFieldTitle         ContactProfileVisibilityField = "TITLE"
	ContactProfileVisibilityFieldStatusMessage ContactProfileVisibilityField = "STATUS_MESSAGE"

	ContactPrivacyExceptionDecisionAllow ContactPrivacyExceptionDecision = "ALLOW"
	ContactPrivacyExceptionDecisionDeny  ContactPrivacyExceptionDecision = "DENY"

	ContactRequestReviewDecisionApprove ContactRequestReviewDecision = "APPROVE"
	ContactRequestReviewDecisionDecline ContactRequestReviewDecision = "DECLINE"
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

type ContactRequestSourcePolicy struct {
	SourceType           ContactRequestSourceType
	AllowContactRequests bool
	RiskLevel            ContactRequestRiskLevel
	ReviewRequired       bool
	Version              int64
	UpdatedAtUnixMS      int64
}

type GetTenantContactRequestSourcePolicyCommand struct {
	TenantID   TenantID
	SourceType ContactRequestSourceType
}

func (c GetTenantContactRequestSourcePolicyCommand) Validate() error {
	if c.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if NormalizeContactRequestSourceType(c.SourceType) == "" {
		return NewInvalidArgument("source_type is invalid")
	}
	return nil
}

func (c GetTenantContactRequestSourcePolicyCommand) NormalizedSourceType() ContactRequestSourceType {
	return NormalizeContactRequestSourceType(c.SourceType)
}

type GetTenantContactRequestSourcePolicyResult struct {
	TenantID TenantID
	Policy   ContactRequestSourcePolicy
}

type SetTenantContactRequestSourcePolicyCommand struct {
	TenantID             TenantID
	SourceType           ContactRequestSourceType
	AllowContactRequests bool
	RiskLevel            ContactRequestRiskLevel
	ReviewRequired       bool
}

func (c SetTenantContactRequestSourcePolicyCommand) Validate() error {
	if c.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if NormalizeContactRequestSourceType(c.SourceType) == "" {
		return NewInvalidArgument("source_type is invalid")
	}
	if NormalizeContactRequestRiskLevel(c.RiskLevel) == "" {
		return NewInvalidArgument("risk_level is invalid")
	}
	return nil
}

func (c SetTenantContactRequestSourcePolicyCommand) NormalizedSourceType() ContactRequestSourceType {
	return NormalizeContactRequestSourceType(c.SourceType)
}

func (c SetTenantContactRequestSourcePolicyCommand) NormalizedRiskLevel() ContactRequestRiskLevel {
	return NormalizeContactRequestRiskLevel(c.RiskLevel)
}

type SetTenantContactRequestSourcePolicyResult struct {
	TenantID TenantID
	Policy   ContactRequestSourcePolicy
	Changed  bool
}

type ReviewContactRequestCommand struct {
	TenantID  TenantID
	RequestID string
	Decision  ContactRequestReviewDecision
	Operator  string
	Reason    string
}

func (c ReviewContactRequestCommand) Validate() error {
	if c.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if strings.TrimSpace(c.RequestID) == "" {
		return NewInvalidArgument("request_id is required")
	}
	if NormalizeContactRequestReviewDecision(c.Decision) == "" {
		return NewInvalidArgument("decision is invalid")
	}
	if strings.TrimSpace(c.Operator) == "" {
		return NewInvalidArgument("operator is required")
	}
	if len(c.Reason) > maxContactReviewReasonLength {
		return NewInvalidArgument("reason is too long")
	}
	return nil
}

func (c ReviewContactRequestCommand) NormalizedDecision() ContactRequestReviewDecision {
	return NormalizeContactRequestReviewDecision(c.Decision)
}

func (c ReviewContactRequestCommand) NormalizedOperator() string {
	return strings.TrimSpace(c.Operator)
}

func NormalizeContactRequestReviewDecision(value ContactRequestReviewDecision) ContactRequestReviewDecision {
	switch ContactRequestReviewDecision(strings.ToUpper(strings.TrimSpace(string(value)))) {
	case ContactRequestReviewDecisionApprove:
		return ContactRequestReviewDecisionApprove
	case ContactRequestReviewDecisionDecline:
		return ContactRequestReviewDecisionDecline
	default:
		return ""
	}
}

type ReviewContactRequestResult struct {
	RequestID      string
	TenantID       TenantID
	SenderUserID   UserID
	ReceiverUserID UserID
	PreviousStatus ContactRequestStatus
	Status         ContactRequestStatus
	Decision       ContactRequestReviewDecision
	RiskLevel      ContactRequestRiskLevel
	ReviewRequired bool
}

type RespondContactRequestCommand struct {
	AuthContext    AuthContext
	RequestID      string
	Decision       ContactDecision
	IdempotencyKey string
}

func (c RespondContactRequestCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if strings.TrimSpace(c.RequestID) == "" {
		return NewInvalidArgument("request_id is required")
	}
	if c.Decision != ContactDecisionAccept && c.Decision != ContactDecisionDecline {
		return NewInvalidArgument("decision is required")
	}
	if strings.TrimSpace(c.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	return nil
}

type RespondContactRequestResult struct {
	RequestID        string
	TenantID         TenantID
	SenderUserID     UserID
	ReceiverUserID   UserID
	Status           ContactRequestStatus
	IdempotentReplay bool
}

type CancelContactRequestCommand struct {
	AuthContext    AuthContext
	RequestID      string
	IdempotencyKey string
}

func (c CancelContactRequestCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if strings.TrimSpace(c.RequestID) == "" {
		return NewInvalidArgument("request_id is required")
	}
	if strings.TrimSpace(c.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	return nil
}

type CancelContactRequestResult struct {
	RequestID        string
	TenantID         TenantID
	SenderUserID     UserID
	ReceiverUserID   UserID
	Status           ContactRequestStatus
	IdempotentReplay bool
}

type ListContactRequestsCommand struct {
	AuthContext          AuthContext
	Direction            ContactRequestListDirection
	Status               ContactRequestStatus
	PageSize             int
	PageToken            string
	SourceTypeFilter     ContactRequestSourceType
	RiskLevelFilter      ContactRequestRiskLevel
	ReviewRequiredFilter *bool
}

func (c ListContactRequestsCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if c.NormalizedDirection() == "" {
		return NewInvalidArgument("direction is invalid")
	}
	if c.NormalizedStatus() == "" {
		return NewInvalidArgument("status is invalid")
	}
	if c.SourceTypeFilter != "" && c.NormalizedSourceTypeFilter() == "" {
		return NewInvalidArgument("source_type_filter is invalid")
	}
	if c.RiskLevelFilter != "" && c.NormalizedRiskLevelFilter() == "" {
		return NewInvalidArgument("risk_level_filter is invalid")
	}
	return nil
}

func (c ListContactRequestsCommand) NormalizedDirection() ContactRequestListDirection {
	if c.Direction == "" {
		return ContactRequestListDirectionIncoming
	}
	if c.Direction == ContactRequestListDirectionIncoming || c.Direction == ContactRequestListDirectionOutgoing {
		return c.Direction
	}
	return ""
}

func (c ListContactRequestsCommand) NormalizedStatus() ContactRequestStatus {
	if c.Status == "" {
		return ContactRequestStatusPending
	}
	switch c.Status {
	case ContactRequestStatusPending,
		ContactRequestStatusReviewRequired,
		ContactRequestStatusAccepted,
		ContactRequestStatusDeclined,
		ContactRequestStatusCanceled,
		ContactRequestStatusExpired:
		return c.Status
	default:
		return ""
	}
}

func (c ListContactRequestsCommand) NormalizedSourceTypeFilter() ContactRequestSourceType {
	switch c.SourceTypeFilter {
	case "":
		return ""
	case ContactRequestSourceTypeDirect,
		ContactRequestSourceTypeSearch,
		ContactRequestSourceTypeGroup,
		ContactRequestSourceTypeInviteLink,
		ContactRequestSourceTypeQRCode,
		ContactRequestSourceTypeImport:
		return c.SourceTypeFilter
	default:
		return ""
	}
}

func (c ListContactRequestsCommand) NormalizedRiskLevelFilter() ContactRequestRiskLevel {
	switch c.RiskLevelFilter {
	case "":
		return ""
	case ContactRequestRiskLevelLow,
		ContactRequestRiskLevelMedium,
		ContactRequestRiskLevelHigh:
		return c.RiskLevelFilter
	default:
		return ""
	}
}

type ContactRequestItem struct {
	RequestID       string
	SenderUserID    UserID
	ReceiverUserID  UserID
	Status          ContactRequestStatus
	Message         string
	CreatedAtUnixMS int64
	UpdatedAtUnixMS int64
	DecidedAtUnixMS int64
	SourceType      ContactRequestSourceType
	SourceRef       string
	RiskLevel       ContactRequestRiskLevel
	ReviewRequired  bool
}

type ListContactRequestsResult struct {
	TenantID      TenantID
	UserID        UserID
	Direction     ContactRequestListDirection
	Status        ContactRequestStatus
	Requests      []ContactRequestItem
	NextPageToken string
}

type ListContactsCommand struct {
	AuthContext AuthContext
	PageSize    int
	PageToken   string
	Query       string
	GroupName   string
}

func (c ListContactsCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if len(c.NormalizedQuery()) > maxContactSearchQueryLength {
		return NewInvalidArgument("query is too long")
	}
	if len(c.NormalizedGroupName()) > maxContactGroupNameLength {
		return NewInvalidArgument("group_name is too long")
	}
	return nil
}

func (c ListContactsCommand) NormalizedQuery() string {
	return strings.TrimSpace(c.Query)
}

func (c ListContactsCommand) NormalizedGroupName() string {
	return strings.TrimSpace(c.GroupName)
}

type ContactItem struct {
	ContactUserID   UserID
	Status          ContactEdgeStatus
	Version         int64
	SourceRequestID string
	CreatedAtUnixMS int64
	UpdatedAtUnixMS int64
	Remark          string
	GroupName       string
}

type ListContactsResult struct {
	TenantID      TenantID
	OwnerUserID   UserID
	Contacts      []ContactItem
	NextPageToken string
}

type GetContactStateCommand struct {
	AuthContext AuthContext
	OtherUserID UserID
}

func (c GetContactStateCommand) Validate() error {
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
	return nil
}

type GetContactStateResult struct {
	TenantID        TenantID
	OwnerUserID     UserID
	ContactUserID   UserID
	Status          ContactEdgeStatus
	SourceRequestID string
	Version         int64
	Remark          string
	GroupName       string
}

type DeleteContactCommand struct {
	AuthContext    AuthContext
	ContactUserID  UserID
	IdempotencyKey string
}

func (c DeleteContactCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if c.ContactUserID == "" {
		return NewInvalidArgument("contact_user_id is required")
	}
	if c.AuthContext.UserID == c.ContactUserID {
		return NewInvalidArgument("contact_user_id must differ from user_id")
	}
	if strings.TrimSpace(c.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	return nil
}

type DeleteContactResult struct {
	TenantID         TenantID
	OwnerUserID      UserID
	ContactUserID    UserID
	Status           ContactEdgeStatus
	SourceRequestID  string
	Version          int64
	IdempotentReplay bool
}

type BlockContactCommand struct {
	AuthContext    AuthContext
	ContactUserID  UserID
	IdempotencyKey string
	Reason         string
}

func (c BlockContactCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if c.ContactUserID == "" {
		return NewInvalidArgument("contact_user_id is required")
	}
	if c.AuthContext.UserID == c.ContactUserID {
		return NewInvalidArgument("contact_user_id must differ from user_id")
	}
	if strings.TrimSpace(c.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if len(c.Reason) > maxContactReasonLength {
		return NewInvalidArgument("reason is too long")
	}
	return nil
}

type BlockContactResult struct {
	TenantID         TenantID
	OwnerUserID      UserID
	ContactUserID    UserID
	Status           ContactEdgeStatus
	SourceRequestID  string
	Version          int64
	IdempotentReplay bool
}

type UnblockContactCommand struct {
	AuthContext    AuthContext
	ContactUserID  UserID
	IdempotencyKey string
}

func (c UnblockContactCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if c.ContactUserID == "" {
		return NewInvalidArgument("contact_user_id is required")
	}
	if c.AuthContext.UserID == c.ContactUserID {
		return NewInvalidArgument("cannot unblock self")
	}
	if strings.TrimSpace(c.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	return nil
}

type UnblockContactResult struct {
	TenantID         TenantID
	OwnerUserID      UserID
	ContactUserID    UserID
	Status           ContactEdgeStatus
	SourceRequestID  string
	Version          int64
	IdempotentReplay bool
}

type UpdateContactRemarkCommand struct {
	AuthContext    AuthContext
	ContactUserID  UserID
	IdempotencyKey string
	Remark         string
}

func (c UpdateContactRemarkCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if c.ContactUserID == "" {
		return NewInvalidArgument("contact_user_id is required")
	}
	if c.AuthContext.UserID == c.ContactUserID {
		return NewInvalidArgument("contact_user_id must differ from user_id")
	}
	if strings.TrimSpace(c.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if len(c.Remark) > maxContactRemarkLength {
		return NewInvalidArgument("remark is too long")
	}
	return nil
}

type UpdateContactRemarkResult struct {
	TenantID         TenantID
	OwnerUserID      UserID
	ContactUserID    UserID
	Status           ContactEdgeStatus
	SourceRequestID  string
	Version          int64
	Remark           string
	IdempotentReplay bool
}

type UpdateContactGroupCommand struct {
	AuthContext    AuthContext
	ContactUserID  UserID
	IdempotencyKey string
	GroupName      string
}

func (c UpdateContactGroupCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if c.ContactUserID == "" {
		return NewInvalidArgument("contact_user_id is required")
	}
	if c.AuthContext.UserID == c.ContactUserID {
		return NewInvalidArgument("contact_user_id must differ from user_id")
	}
	if strings.TrimSpace(c.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if len(c.NormalizedGroupName()) > maxContactGroupNameLength {
		return NewInvalidArgument("group_name is too long")
	}
	return nil
}

func (c UpdateContactGroupCommand) NormalizedGroupName() string {
	return strings.TrimSpace(c.GroupName)
}

type UpdateContactGroupResult struct {
	TenantID         TenantID
	OwnerUserID      UserID
	ContactUserID    UserID
	Status           ContactEdgeStatus
	SourceRequestID  string
	Version          int64
	GroupName        string
	IdempotentReplay bool
}
