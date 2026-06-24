package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	"github.com/qsyy0921/IM/services/action-executor/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ConversationProfileExecutor struct {
	client  conversationv1.ConversationServiceClient
	timeout time.Duration
}

func NewConversationProfileExecutor(
	client conversationv1.ConversationServiceClient,
	timeout time.Duration,
) ConversationProfileExecutor {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	return ConversationProfileExecutor{client: client, timeout: timeout}
}

func (executor ConversationProfileExecutor) ExecuteTool(
	ctx context.Context,
	command types.ToolExecutionCommand,
) (types.ToolExecutionResult, error) {
	if executor.client == nil {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	if strings.TrimSpace(command.ToolName) != types.ConversationProfileUpdateToolName {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	if strings.TrimSpace(command.ResourceType) != "conversation" {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	if strings.ToUpper(strings.TrimSpace(command.RiskLevel)) != "LOW" {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	if !types.ToolActionAllowed(command.Skill.AllowedActions, types.ToolActionExecute) {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	if strings.TrimSpace(command.ResourceID) == "" {
		return types.ToolExecutionResult{}, types.ErrToolExecutionFailed
	}
	input, err := decodeConversationProfileInput(command.InputJSON)
	if err != nil {
		return types.ToolExecutionResult{}, err
	}

	callCtx, cancel := context.WithTimeout(ctx, executor.timeout)
	defer cancel()
	response, err := executor.client.UpdateConversationProfile(callCtx, &conversationv1.UpdateConversationProfileRequest{
		AuthContext: &conversationv1.AuthContext{
			TenantId:  string(command.AuthContext.TenantID),
			UserId:    string(command.AuthContext.UserID),
			DeviceId:  command.AuthContext.DeviceID,
			SessionId: command.AuthContext.SessionID,
			TraceId:   command.AuthContext.TraceID,
			RequestId: command.AuthContext.RequestID,
		},
		ConversationId:         command.ResourceID,
		Title:                  input.Title,
		AvatarUri:              input.AvatarURI,
		Announcement:           input.Announcement,
		ExpectedProfileVersion: input.ExpectedProfileVersion,
	})
	if err != nil {
		return types.ToolExecutionResult{}, mapConversationProfileError(err)
	}
	profile := response.GetProfile()
	output, err := json.Marshal(map[string]any{
		"schema_version":      1,
		"adapter":             "conversation-profile",
		"tool_name":           command.ToolName,
		"resource_type":       command.ResourceType,
		"resource_id":         command.ResourceID,
		"input_sha256":        command.InputSHA256,
		"status":              "updated",
		"conversation_id":     profile.GetConversationId(),
		"profile_version":     profile.GetProfileVersion(),
		"title_sha256":        sha256Hex(input.Title),
		"avatar_uri_sha256":   sha256Hex(input.AvatarURI),
		"announcement_sha256": sha256Hex(input.Announcement),
	})
	if err != nil {
		return types.ToolExecutionResult{}, types.ErrToolExecutionFailed
	}
	return types.ToolExecutionResult{
		Executed:   true,
		OutputJSON: string(output),
	}, nil
}

type conversationProfileInput struct {
	Title                  string `json:"title"`
	AvatarURI              string `json:"avatar_uri"`
	Announcement           string `json:"announcement"`
	ExpectedProfileVersion int64  `json:"expected_profile_version"`
}

func decodeConversationProfileInput(inputJSON string) (conversationProfileInput, error) {
	if strings.TrimSpace(inputJSON) == "" {
		return conversationProfileInput{}, types.ErrToolExecutionFailed
	}
	var input conversationProfileInput
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return conversationProfileInput{}, types.ErrToolExecutionFailed
	}
	input.Title = strings.TrimSpace(input.Title)
	input.AvatarURI = strings.TrimSpace(input.AvatarURI)
	input.Announcement = strings.TrimSpace(input.Announcement)
	if input.Title == "" || len(input.Title) > 128 ||
		len(input.AvatarURI) > 512 ||
		len(input.Announcement) > 1024 ||
		input.ExpectedProfileVersion <= 0 {
		return conversationProfileInput{}, types.ErrToolExecutionFailed
	}
	return input, nil
}

func mapConversationProfileError(err error) error {
	switch status.Code(err) {
	case codes.PermissionDenied:
		return types.ErrToolProviderPermissionDenied
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.ErrToolProviderUnavailable
	case codes.ResourceExhausted:
		return types.ErrToolProviderRateLimited
	default:
		return types.ErrToolExecutionFailed
	}
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
