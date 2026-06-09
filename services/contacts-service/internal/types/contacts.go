package types

import "strings"

type ContactRequestStatus string
type ContactEdgeStatus string
type ContactDecision string

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
)

type SendContactRequestCommand struct {
	AuthContext    AuthContext
	TargetUserID   UserID
	IdempotencyKey string
	Message        string
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
	return nil
}

type SendContactRequestResult struct {
	RequestID        string
	TenantID         TenantID
	SenderUserID     UserID
	ReceiverUserID   UserID
	Status           ContactRequestStatus
	IdempotentReplay bool
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

type ListContactsCommand struct {
	AuthContext AuthContext
	PageSize    int
	PageToken   string
}

func (c ListContactsCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	return nil
}

type ContactItem struct {
	ContactUserID   UserID
	Status          ContactEdgeStatus
	Version         int64
	SourceRequestID string
	CreatedAtUnixMS int64
	UpdatedAtUnixMS int64
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
}
