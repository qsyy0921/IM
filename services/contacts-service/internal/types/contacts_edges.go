package types

import "strings"

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
