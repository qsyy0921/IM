package types

import "strings"

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
