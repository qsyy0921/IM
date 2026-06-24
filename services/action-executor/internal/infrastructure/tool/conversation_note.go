package tool

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	"github.com/qsyy0921/IM/services/action-executor/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ConversationNoteExecutor struct {
	client  conversationv1.ConversationServiceClient
	timeout time.Duration
}

func NewConversationNoteExecutor(
	client conversationv1.ConversationServiceClient,
	timeout time.Duration,
) ConversationNoteExecutor {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	return ConversationNoteExecutor{client: client, timeout: timeout}
}

func (executor ConversationNoteExecutor) ExecuteTool(
	ctx context.Context,
	command types.ToolExecutionCommand,
) (types.ToolExecutionResult, error) {
	if executor.client == nil {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	if strings.TrimSpace(command.ToolName) != types.ConversationNoteCreateToolName {
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
	input, err := decodeConversationNoteInput(command.InputJSON)
	if err != nil {
		return types.ToolExecutionResult{}, err
	}
	idempotencyKey := strings.TrimSpace(command.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = command.ProposalID + ":" + command.ApprovalID + ":" + command.InputSHA256
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return types.ToolExecutionResult{}, types.ErrToolExecutionFailed
	}

	callCtx, cancel := context.WithTimeout(ctx, executor.timeout)
	defer cancel()
	response, err := executor.client.CreateConversationNote(callCtx, &conversationv1.CreateConversationNoteRequest{
		AuthContext: &conversationv1.AuthContext{
			TenantId:  string(command.AuthContext.TenantID),
			UserId:    string(command.AuthContext.UserID),
			DeviceId:  command.AuthContext.DeviceID,
			SessionId: command.AuthContext.SessionID,
			TraceId:   command.AuthContext.TraceID,
			RequestId: command.AuthContext.RequestID,
		},
		ConversationId:   command.ResourceID,
		Body:             input.Body,
		IdempotencyKey:   idempotencyKey,
		SourceToolName:   command.ToolName,
		SourceProposalId: command.ProposalID,
		SourceApprovalId: command.ApprovalID,
	})
	if err != nil {
		return types.ToolExecutionResult{}, mapConversationNoteError(err)
	}
	note := response.GetNote()
	output, err := json.Marshal(map[string]any{
		"schema_version":    1,
		"adapter":           "conversation-note",
		"tool_name":         command.ToolName,
		"resource_type":     command.ResourceType,
		"resource_id":       command.ResourceID,
		"input_sha256":      command.InputSHA256,
		"status":            "created",
		"conversation_id":   note.GetConversationId(),
		"note_id":           note.GetNoteId(),
		"note_ref":          "conversation://" + note.GetConversationId() + "/notes/" + note.GetNoteId(),
		"idempotent_replay": response.GetIdempotentReplay(),
	})
	if err != nil {
		return types.ToolExecutionResult{}, types.ErrToolExecutionFailed
	}
	return types.ToolExecutionResult{
		Executed:   true,
		OutputJSON: string(output),
	}, nil
}

type conversationNoteInput struct {
	Body string `json:"body"`
}

func decodeConversationNoteInput(inputJSON string) (conversationNoteInput, error) {
	if strings.TrimSpace(inputJSON) == "" {
		return conversationNoteInput{}, types.ErrToolExecutionFailed
	}
	var input conversationNoteInput
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return conversationNoteInput{}, types.ErrToolExecutionFailed
	}
	input.Body = strings.TrimSpace(input.Body)
	if input.Body == "" || len(input.Body) > 4096 {
		return conversationNoteInput{}, types.ErrToolExecutionFailed
	}
	return input, nil
}

func mapConversationNoteError(err error) error {
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
