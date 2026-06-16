package types

import (
	"strings"
	"unicode"
)

const (
	maxContactRemarkLength      = 128
	maxContactGroupNameLength   = 64
	maxContactReasonLength      = 512
	maxContactSearchQueryLength = 128
	maxContactSourceRefLength   = 256
)

type ContactRequestStatus string
type ContactEdgeStatus string
type ContactDecision string
type ContactRequestListDirection string
type ContactRequestSourceType string
type ContactPrivacyPolicySource string

const (
	ContactRequestStatusPending  ContactRequestStatus = "PENDING"
	ContactRequestStatusAccepted ContactRequestStatus = "ACCEPTED"
	ContactRequestStatusDeclined ContactRequestStatus = "DECLINED"
	ContactRequestStatusCanceled ContactRequestStatus = "CANCELED"
	ContactRequestStatusExpired  ContactRequestStatus = "EXPIRED"

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

	ContactPrivacyPolicySourceUser          ContactPrivacyPolicySource = "USER"
	ContactPrivacyPolicySourceTenantDefault ContactPrivacyPolicySource = "TENANT_DEFAULT"
	ContactPrivacyPolicySourceSystemDefault ContactPrivacyPolicySource = "SYSTEM_DEFAULT"
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

type SendContactRequestResult struct {
	RequestID        string
	TenantID         TenantID
	SenderUserID     UserID
	ReceiverUserID   UserID
	Status           ContactRequestStatus
	IdempotentReplay bool
	SourceType       ContactRequestSourceType
	SourceRef        string
}

type ContactPrivacySettings struct {
	AllowContactRequests       bool
	AllowSearchContactRequests bool
	AllowProfileVisibility     bool
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
	AuthContext                AuthContext
	AllowContactRequests       bool
	AllowSearchContactRequests *bool
	AllowProfileVisibility     *bool
	IdempotencyKey             string
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
	return nil
}

type SetContactPrivacyResult struct {
	TenantID         TenantID
	UserID           UserID
	Settings         ContactPrivacySettings
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
	TenantID                   TenantID
	AllowContactRequests       bool
	AllowSearchContactRequests *bool
	AllowProfileVisibility     *bool
}

func (c SetTenantContactPrivacyDefaultCommand) Validate() error {
	if c.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	return nil
}

type SetTenantContactPrivacyDefaultResult struct {
	TenantID TenantID
	Settings ContactPrivacySettings
	Changed  bool
}

type ContactRequestSourcePolicy struct {
	SourceType           ContactRequestSourceType
	AllowContactRequests bool
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
}

func (c SetTenantContactRequestSourcePolicyCommand) Validate() error {
	if c.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if NormalizeContactRequestSourceType(c.SourceType) == "" {
		return NewInvalidArgument("source_type is invalid")
	}
	return nil
}

func (c SetTenantContactRequestSourcePolicyCommand) NormalizedSourceType() ContactRequestSourceType {
	return NormalizeContactRequestSourceType(c.SourceType)
}

type SetTenantContactRequestSourcePolicyResult struct {
	TenantID TenantID
	Policy   ContactRequestSourcePolicy
	Changed  bool
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
	AuthContext AuthContext
	Direction   ContactRequestListDirection
	Status      ContactRequestStatus
	PageSize    int
	PageToken   string
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
		ContactRequestStatusAccepted,
		ContactRequestStatusDeclined,
		ContactRequestStatusCanceled,
		ContactRequestStatusExpired:
		return c.Status
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
