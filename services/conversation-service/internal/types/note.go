package types

import (
	"strings"
	"time"
)

const (
	MaxConversationNoteBodyLength        = 4096
	MaxConversationNoteSourceFieldLength = 160
)

type NoteID string

type CreateConversationNoteCommand struct {
	AuthContext       AuthContext
	ConversationID    ConversationID
	Body              string
	IdempotencyKey    string
	SourceToolName    string
	SourceProposalID  string
	SourceApprovalID  string
	SourceExecutionID string
}

func (command CreateConversationNoteCommand) Validate() error {
	if command.AuthContext.TenantID == "" {
		return NewInvalidArgument("auth_context.tenant_id is required")
	}
	if command.AuthContext.UserID == "" {
		return NewInvalidArgument("auth_context.user_id is required")
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if command.NormalizedBody() == "" {
		return NewInvalidArgument("body is required")
	}
	if len(command.NormalizedBody()) > MaxConversationNoteBodyLength {
		return NewInvalidArgument("body is too long")
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if len(strings.TrimSpace(command.IdempotencyKey)) > MaxConversationNoteSourceFieldLength {
		return NewInvalidArgument("idempotency_key is too long")
	}
	for _, value := range []string{
		command.SourceToolName,
		command.SourceProposalID,
		command.SourceApprovalID,
		command.SourceExecutionID,
	} {
		if len(strings.TrimSpace(value)) > MaxConversationNoteSourceFieldLength {
			return NewInvalidArgument("source field is too long")
		}
	}
	if containsNUL(command.Body) ||
		containsNUL(command.IdempotencyKey) ||
		containsNUL(command.SourceToolName) ||
		containsNUL(command.SourceProposalID) ||
		containsNUL(command.SourceApprovalID) ||
		containsNUL(command.SourceExecutionID) {
		return NewInvalidArgument("note contains unsupported characters")
	}
	return nil
}

func (command CreateConversationNoteCommand) NormalizedBody() string {
	return strings.TrimSpace(command.Body)
}

func (command CreateConversationNoteCommand) NormalizedIdempotencyKey() string {
	return strings.TrimSpace(command.IdempotencyKey)
}

func (command CreateConversationNoteCommand) NormalizedSourceToolName() string {
	return strings.TrimSpace(command.SourceToolName)
}

func (command CreateConversationNoteCommand) NormalizedSourceProposalID() string {
	return strings.TrimSpace(command.SourceProposalID)
}

func (command CreateConversationNoteCommand) NormalizedSourceApprovalID() string {
	return strings.TrimSpace(command.SourceApprovalID)
}

func (command CreateConversationNoteCommand) NormalizedSourceExecutionID() string {
	return strings.TrimSpace(command.SourceExecutionID)
}

type ConversationNoteResult struct {
	TenantID          TenantID
	ConversationID    ConversationID
	NoteID            NoteID
	AuthorUserID      UserID
	Body              string
	SourceToolName    string
	SourceProposalID  string
	SourceApprovalID  string
	SourceExecutionID string
	IdempotentReplay  bool
	CreatedAt         time.Time
}
