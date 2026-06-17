package types

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
