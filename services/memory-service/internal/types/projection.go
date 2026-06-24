package types

import "strings"

const (
	TimelineEventMessagePersisted = "message.persisted.v1"
	TimelineEventMessageEdited    = "message.edited.v1"
	TimelineEventMessageRevoked   = "message.revoked.v1"
	TimelineEventMessageDeleted   = "message.deleted.v1"

	TimelineEventConversationMemberJoined            = "conversation.member.joined.v1"
	TimelineEventConversationMemberLeft              = "conversation.member.left.v1"
	TimelineEventConversationMemberRemoved           = "conversation.member.removed.v1"
	TimelineEventConversationMemberRoleChanged       = "conversation.member.role_changed.v1"
	TimelineEventConversationMemberBoundaryCancelled = "conversation.member.boundary_cancelled.v1"
	TimelineEventConversationMemberOwnerTransferred  = "conversation.member.owner_transferred.v1"

	MemoryMemberRoleOwner  = "OWNER"
	MemoryMemberRoleAdmin  = "ADMIN"
	MemoryMemberRoleMember = "MEMBER"

	MemoryMemberStatusActive  = "ACTIVE"
	MemoryMemberStatusLeft    = "LEFT"
	MemoryMemberStatusRemoved = "REMOVED"
	MemoryMemberStatusBanned  = "BANNED"
)

type TimelineMessage struct {
	Topic     string
	Partition int
	Offset    int64
	Key       []byte
	Value     []byte
}

type ProjectTimelineEventCommand struct {
	TenantID        TenantID
	EventID         string
	EventType       string
	ConversationID  ConversationID
	ConversationSeq int64

	MessageID         string
	SenderID          UserID
	ProjectMemory     bool
	MemoryEventType   string
	MemoryReviewState string
	MemoryConfidence  float64
	FactText          string
	TopicText         string
	MemoryEventID     string
	ExtractionVersion string

	TargetUserID        UserID
	MemberRole          string
	MemberStatus        string
	MemberVersion       int64
	PermissionVersion   int64
	PreviousOwnerUserID UserID
	PreviousOwnerRole   string
	PreviousOwnerStatus string
	NewOwnerUserID      UserID
	NewOwnerRole        string
	NewOwnerStatus      string

	ConsumerGroup string
	Topic         string
	PartitionID   int32
	OffsetValue   int64
	TraceID       string
	CorrelationID string
	CausationID   string
}

func (command ProjectTimelineEventCommand) Validate() error {
	if strings.TrimSpace(string(command.TenantID)) == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if strings.TrimSpace(command.EventID) == "" {
		return NewInvalidArgument("event_id is required")
	}
	if strings.TrimSpace(command.EventType) == "" {
		return NewInvalidArgument("event_type is required")
	}
	if strings.TrimSpace(string(command.ConversationID)) == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if command.ConversationSeq <= 0 {
		return NewInvalidArgument("conversation_seq is required")
	}
	if command.ConsumerGroup == "" || command.Topic == "" || command.PartitionID < 0 || command.OffsetValue < 0 {
		return NewInvalidArgument("checkpoint fields are required")
	}
	switch command.EventType {
	case TimelineEventMessagePersisted, TimelineEventMessageEdited:
		if strings.TrimSpace(command.MessageID) == "" {
			return NewInvalidArgument("message_id is required")
		}
		if command.ProjectMemory {
			if strings.TrimSpace(command.FactText) == "" {
				return NewInvalidArgument("fact_text is required")
			}
			if !isValidMemoryEventType(command.MemoryEventType) {
				return NewInvalidArgument("invalid memory event type")
			}
			if !isValidMemoryReviewState(command.MemoryReviewState) {
				return NewInvalidArgument("invalid memory review state")
			}
			if command.MemoryConfidence < 0 || command.MemoryConfidence > 1 {
				return NewInvalidArgument("memory confidence must be between 0 and 1")
			}
		}
	case TimelineEventMessageRevoked, TimelineEventMessageDeleted:
		if strings.TrimSpace(command.MessageID) == "" {
			return NewInvalidArgument("message_id is required")
		}
	case TimelineEventConversationMemberJoined, TimelineEventConversationMemberLeft, TimelineEventConversationMemberRemoved, TimelineEventConversationMemberRoleChanged:
		if strings.TrimSpace(string(command.TargetUserID)) == "" {
			return NewInvalidArgument("target_user_id is required")
		}
	case TimelineEventConversationMemberOwnerTransferred:
		if command.PreviousOwnerUserID == "" || command.NewOwnerUserID == "" {
			return NewInvalidArgument("owner transfer users are required")
		}
	case TimelineEventConversationMemberBoundaryCancelled:
		return nil
	default:
		return NewUnsupportedPayload("unsupported timeline event type")
	}
	return nil
}

type ProjectTimelineEventResult struct {
	TenantID        TenantID
	EventID         string
	ConversationID  ConversationID
	ConversationSeq int64
	ProjectedMemory bool
	ProjectedMember bool
}
