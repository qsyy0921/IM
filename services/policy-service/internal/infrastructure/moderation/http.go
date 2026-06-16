package moderation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type HTTPConfig struct {
	Endpoint          string
	BearerToken       string
	Timeout           time.Duration
	PermissionVersion int64
	Classification    string
	Reason            string
	Client            *http.Client
}

type HTTPModerator struct {
	endpoint          string
	bearerToken       string
	timeout           time.Duration
	permissionVersion int64
	classification    string
	reason            string
	client            *http.Client
}

type httpModerationRequest struct {
	TenantID       string              `json:"tenant_id"`
	UserID         string              `json:"user_id"`
	ConversationID string              `json:"conversation_id"`
	MessageID      string              `json:"message_id,omitempty"`
	Action         types.MessageAction `json:"action"`
	MessageText    string              `json:"message_text"`
	TraceID        string              `json:"trace_id,omitempty"`
	RequestID      string              `json:"request_id,omitempty"`
}

type httpModerationResponse struct {
	Allowed           *bool  `json:"allowed"`
	PermissionVersion int64  `json:"permission_version"`
	Classification    string `json:"classification"`
	Reason            string `json:"reason"`
}

func NewHTTPModerator(config HTTPConfig) (HTTPModerator, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		return HTTPModerator{}, errors.New("moderation HTTP endpoint is required")
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = time.Second
	}
	permissionVersion := config.PermissionVersion
	if permissionVersion <= 0 {
		permissionVersion = 1
	}
	classification := strings.TrimSpace(config.Classification)
	if classification == "" {
		classification = "CONTENT_PROVIDER_DENIED"
	}
	reason := strings.TrimSpace(config.Reason)
	if reason == "" {
		reason = "content moderation provider denied"
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return HTTPModerator{
		endpoint:          endpoint,
		bearerToken:       strings.TrimSpace(config.BearerToken),
		timeout:           timeout,
		permissionVersion: permissionVersion,
		classification:    classification,
		reason:            reason,
		client:            client,
	}, nil
}

func (m HTTPModerator) ModerateMessageContent(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, bool, error) {
	text := strings.TrimSpace(command.MessageText)
	if text == "" {
		return types.MessageActionDecision{}, false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	body, err := json.Marshal(httpModerationRequest{
		TenantID:       string(command.AuthContext.TenantID),
		UserID:         string(command.AuthContext.UserID),
		ConversationID: string(command.ConversationID),
		MessageID:      string(command.MessageID),
		Action:         command.Action,
		MessageText:    text,
		TraceID:        strings.TrimSpace(command.AuthContext.TraceID),
		RequestID:      strings.TrimSpace(command.AuthContext.RequestID),
	})
	if err != nil {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("content moderation provider unavailable")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint, bytes.NewReader(body))
	if err != nil {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("content moderation provider unavailable")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if m.bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+m.bearerToken)
	}
	response, err := m.client.Do(request)
	if err != nil {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("content moderation provider unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("content moderation provider unavailable")
	}
	limited := io.LimitReader(response.Body, 64*1024)
	var result httpModerationResponse
	if err := json.NewDecoder(limited).Decode(&result); err != nil {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("content moderation provider unavailable")
	}
	if result.Allowed == nil {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("content moderation provider unavailable")
	}
	if *result.Allowed {
		return types.MessageActionDecision{}, false, nil
	}
	classification := strings.TrimSpace(result.Classification)
	if classification == "" {
		classification = m.classification
	}
	reason := strings.TrimSpace(result.Reason)
	if reason == "" {
		reason = m.reason
	}
	permissionVersion := result.PermissionVersion
	if permissionVersion <= 0 {
		permissionVersion = m.permissionVersion
	}
	return types.MessageActionDecision{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ConversationID:    command.ConversationID,
		MessageID:         command.MessageID,
		Action:            command.Action,
		Allowed:           false,
		PermissionVersion: permissionVersion,
		Classification:    classification,
		Reason:            reason,
	}, true, nil
}

func (m HTTPModerator) String() string {
	return fmt.Sprintf("http moderation provider endpoint=%s", m.endpoint)
}
