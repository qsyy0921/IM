package types

import (
	"strings"
	"time"
)

const (
	MaxConversationProfileTitleLength        = 128
	MaxConversationProfileAvatarURILength    = 512
	MaxConversationProfileAnnouncementLength = 1024
)

type GetConversationProfileCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
}

func (c GetConversationProfileCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("auth_context.tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("auth_context.user_id is required")
	}
	if c.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	return nil
}

type UpdateConversationProfileCommand struct {
	AuthContext            AuthContext
	ConversationID         ConversationID
	Title                  string
	AvatarURI              string
	Announcement           string
	ExpectedProfileVersion int64
}

func (c UpdateConversationProfileCommand) Validate() error {
	if c.AuthContext.TenantID == "" {
		return NewInvalidArgument("auth_context.tenant_id is required")
	}
	if c.AuthContext.UserID == "" {
		return NewInvalidArgument("auth_context.user_id is required")
	}
	if c.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if c.ExpectedProfileVersion < 0 {
		return NewInvalidArgument("expected_profile_version is invalid")
	}
	if c.NormalizedTitle() == "" {
		return NewInvalidArgument("title is required")
	}
	if len(c.NormalizedTitle()) > MaxConversationProfileTitleLength {
		return NewInvalidArgument("title is too long")
	}
	if len(c.NormalizedAvatarURI()) > MaxConversationProfileAvatarURILength {
		return NewInvalidArgument("avatar_uri is too long")
	}
	if len(c.NormalizedAnnouncement()) > MaxConversationProfileAnnouncementLength {
		return NewInvalidArgument("announcement is too long")
	}
	if containsNUL(c.Title) || containsNUL(c.AvatarURI) || containsNUL(c.Announcement) {
		return NewInvalidArgument("profile contains unsupported characters")
	}
	return nil
}

func (c UpdateConversationProfileCommand) NormalizedTitle() string {
	return strings.TrimSpace(c.Title)
}

func (c UpdateConversationProfileCommand) NormalizedAvatarURI() string {
	return strings.TrimSpace(c.AvatarURI)
}

func (c UpdateConversationProfileCommand) NormalizedAnnouncement() string {
	return strings.TrimSpace(c.Announcement)
}

type ConversationProfileResult struct {
	TenantID          TenantID
	ConversationID    ConversationID
	ConversationType  ConversationType
	Title             string
	AvatarURI         string
	Announcement      string
	ProfileVersion    int64
	MemberVersion     int64
	PermissionVersion int64
	UpdatedAt         time.Time
}

func containsNUL(value string) bool {
	for _, char := range value {
		if char == 0 {
			return true
		}
	}
	return false
}
