package postgres

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type contactPageCursor struct {
	Version       int            `json:"v"`
	TenantID      types.TenantID `json:"tenant_id"`
	OwnerUserID   types.UserID   `json:"owner_user_id"`
	PageSize      int            `json:"page_size"`
	Query         string         `json:"query,omitempty"`
	GroupName     string         `json:"group_name,omitempty"`
	ContactUserID string         `json:"contact_user_id"`
}

type contactRequestPageCursor struct {
	Version              int                               `json:"v"`
	TenantID             types.TenantID                    `json:"tenant_id"`
	UserID               types.UserID                      `json:"user_id"`
	Direction            types.ContactRequestListDirection `json:"direction"`
	Status               types.ContactRequestStatus        `json:"status"`
	SourceTypeFilter     types.ContactRequestSourceType    `json:"source_type_filter,omitempty"`
	RiskLevelFilter      types.ContactRequestRiskLevel     `json:"risk_level_filter,omitempty"`
	ReviewRequiredFilter *bool                             `json:"review_required_filter,omitempty"`
	PageSize             int                               `json:"page_size"`
	CreatedAt            time.Time                         `json:"created_at"`
	RequestID            string                            `json:"request_id"`
}

type contactPrivacyExceptionPageCursor struct {
	Version     int            `json:"v"`
	TenantID    types.TenantID `json:"tenant_id"`
	OwnerUserID types.UserID   `json:"owner_user_id"`
	PageSize    int            `json:"page_size"`
	OtherUserID string         `json:"other_user_id"`
}

func decodeContactRequestPageTokenFor(
	command types.ListContactRequestsCommand,
	direction types.ContactRequestListDirection,
	status types.ContactRequestStatus,
	sourceTypeFilter types.ContactRequestSourceType,
	riskLevelFilter types.ContactRequestRiskLevel,
	reviewRequiredFilter *bool,
	pageSize int,
) (contactRequestPageCursor, bool, error) {
	value := command.PageToken
	if value == "" {
		return contactRequestPageCursor{}, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return contactRequestPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	var cursor contactRequestPageCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return contactRequestPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	if cursor.Version != 1 && cursor.Version != 2 || cursor.RequestID == "" || cursor.CreatedAt.IsZero() {
		return contactRequestPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	if cursor.TenantID != command.AuthContext.TenantID ||
		cursor.UserID != command.AuthContext.UserID ||
		cursor.Direction != direction ||
		cursor.Status != status ||
		cursor.PageSize != pageSize {
		return contactRequestPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	if cursor.Version == 1 {
		if sourceTypeFilter != "" || riskLevelFilter != "" || reviewRequiredFilter != nil {
			return contactRequestPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
		}
		return cursor, true, nil
	}
	if cursor.SourceTypeFilter != sourceTypeFilter ||
		cursor.RiskLevelFilter != riskLevelFilter ||
		!boolPointerEqual(cursor.ReviewRequiredFilter, reviewRequiredFilter) {
		return contactRequestPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	return cursor, true, nil
}

func boolPointerEqual(left *bool, right *bool) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func encodeContactRequestPageToken(cursor contactRequestPageCursor) string {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodePageTokenFor(command types.ListContactsCommand, pageSize int) (contactPageCursor, bool, error) {
	value := command.PageToken
	if value == "" {
		return contactPageCursor{}, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return contactPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	var cursor contactPageCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return contactPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	if cursor.Version != 1 || cursor.ContactUserID == "" {
		return contactPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	if cursor.TenantID != command.AuthContext.TenantID ||
		cursor.OwnerUserID != command.AuthContext.UserID ||
		cursor.PageSize != pageSize ||
		cursor.Query != command.NormalizedQuery() ||
		cursor.GroupName != command.NormalizedGroupName() {
		return contactPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	return cursor, true, nil
}

func encodePageToken(cursor contactPageCursor) string {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeContactPrivacyExceptionPageTokenFor(command types.ListContactPrivacyExceptionsCommand, pageSize int) (contactPrivacyExceptionPageCursor, bool, error) {
	value := command.PageToken
	if value == "" {
		return contactPrivacyExceptionPageCursor{}, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return contactPrivacyExceptionPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	var cursor contactPrivacyExceptionPageCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return contactPrivacyExceptionPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	if cursor.Version != 1 || cursor.OtherUserID == "" {
		return contactPrivacyExceptionPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	if cursor.TenantID != command.AuthContext.TenantID ||
		cursor.OwnerUserID != command.AuthContext.UserID ||
		cursor.PageSize != pageSize {
		return contactPrivacyExceptionPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	return cursor, true, nil
}

func encodeContactPrivacyExceptionPageToken(cursor contactPrivacyExceptionPageCursor) string {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
