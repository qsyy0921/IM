package types

import "strings"

const (
	maxContactRemarkLength = 128
	maxContactReasonLength = 512
)

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
	Remark          string
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
