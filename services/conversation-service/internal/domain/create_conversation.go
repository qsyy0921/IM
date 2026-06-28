package domain

import (
	"encoding/json"
	"time"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type ConversationCreateRecord struct {
	Conversation Conversation
	Members      []MemberMutation
	Timeline     []TimelineEvent
	Outbox       []OutboxEvent
	BoundarySeq  int64
}

func NewConversationCreateRecord(
	command types.CreateConversationCommand,
	eventIDs []types.EventID,
	boundarySeq int64,
	now time.Time,
) (ConversationCreateRecord, error) {
	if command.ConversationType != types.ConversationTypeGroup &&
		command.ConversationType != types.ConversationTypeDirect {
		return ConversationCreateRecord{}, types.NewInvalidArgument("conversation_type is not supported")
	}
	if boundarySeq <= 0 {
		return ConversationCreateRecord{}, types.NewInvalidArgument("boundary_seq is invalid")
	}
	members := buildInitialConversationMembers(command, boundarySeq)
	if len(members) == 0 {
		return ConversationCreateRecord{}, types.NewInvalidArgument("conversation members are required")
	}
	if len(eventIDs) != len(members) {
		return ConversationCreateRecord{}, types.NewInvalidArgument("event_ids count must match members")
	}
	policy, err := ResolveConversationCreatePolicy(command.ConversationType, int64(len(members)))
	if err != nil {
		return ConversationCreateRecord{}, err
	}
	memberVersion := int64(len(members))
	permissionVersion := memberVersion
	occurredAt := now.UTC()
	conversation := Conversation{
		TenantID:            command.AuthContext.TenantID,
		ConversationID:      command.ConversationID,
		ConversationType:    command.ConversationType,
		Status:              types.ConversationStatusActive,
		ConversationMode:    policy.ConversationMode,
		FanoutMode:          policy.FanoutMode,
		FanoutPolicyVersion: policy.FanoutPolicyVersion,
		MemberVersion:       memberVersion,
		PermissionVersion:   permissionVersion,
		CurrentSeqShard:     policy.CurrentSeqShard,
		DirectPeerUserID:    command.DirectPeerUserID,
	}
	timeline, outbox, err := buildConversationCreateBoundaryEvents(command, members, eventIDs, policy, occurredAt)
	if err != nil {
		return ConversationCreateRecord{}, err
	}
	return ConversationCreateRecord{
		Conversation: conversation,
		Members:      members,
		Timeline:     timeline,
		Outbox:       outbox,
		BoundarySeq:  boundarySeq + int64(len(members)) - 1,
	}, nil
}

func buildInitialConversationMembers(command types.CreateConversationCommand, boundarySeq int64) []MemberMutation {
	switch command.ConversationType {
	case types.ConversationTypeGroup:
		seq := boundarySeq
		return []MemberMutation{{
			UserID:            command.AuthContext.UserID,
			NewRole:           types.MemberRoleOwner,
			NewStatus:         types.MemberStatusActive,
			MemberVersion:     1,
			PermissionVersion: 1,
			JoinSeq:           &seq,
		}}
	case types.ConversationTypeDirect:
		firstSeq := boundarySeq
		secondSeq := boundarySeq + 1
		return []MemberMutation{
			{
				UserID:            command.AuthContext.UserID,
				NewRole:           types.MemberRoleMember,
				NewStatus:         types.MemberStatusActive,
				MemberVersion:     1,
				PermissionVersion: 1,
				JoinSeq:           &firstSeq,
			},
			{
				UserID:            command.DirectPeerUserID,
				NewRole:           types.MemberRoleMember,
				NewStatus:         types.MemberStatusActive,
				MemberVersion:     2,
				PermissionVersion: 2,
				JoinSeq:           &secondSeq,
			},
		}
	default:
		return nil
	}
}

func buildConversationCreateBoundaryEvents(
	command types.CreateConversationCommand,
	members []MemberMutation,
	eventIDs []types.EventID,
	policy ConversationScalePolicy,
	occurredAt time.Time,
) ([]TimelineEvent, []OutboxEvent, error) {
	traceID := firstNonEmpty(command.AuthContext.TraceID, command.AuthContext.RequestID)
	partitionKey := string(command.AuthContext.TenantID) + ":" + string(command.ConversationID)
	eventType := types.TimelineEventConversationMemberJoined
	timeline := make([]TimelineEvent, 0, len(members))
	outbox := make([]OutboxEvent, 0, len(members))
	for index, member := range members {
		seq := int64(0)
		if member.JoinSeq != nil {
			seq = *member.JoinSeq
		}
		eventID := eventIDs[index]
		payloadJSON, err := json.Marshal(map[string]any{
			"change_id":          eventID,
			"conversation_id":    command.ConversationID,
			"boundary_seq":       seq,
			"target_user_id":     member.UserID,
			"operator_user_id":   command.AuthContext.UserID,
			"change_type":        types.MemberChangeTypeJoin,
			"old_role":           "",
			"new_role":           member.NewRole,
			"old_status":         "",
			"new_status":         types.MemberStatusActive,
			"member_version":     member.MemberVersion,
			"permission_version": member.PermissionVersion,
			"reason":             conversationCreateReason(command.ConversationType),
			"occurred_at":        occurredAt.Format(time.RFC3339Nano),
		})
		if err != nil {
			return nil, nil, err
		}
		timeline = append(timeline, TimelineEvent{
			EventID:             eventID,
			EventType:           eventType,
			EventVersion:        "v1",
			TenantID:            command.AuthContext.TenantID,
			ConversationID:      command.ConversationID,
			ConversationSeq:     seq,
			ActorID:             command.AuthContext.UserID,
			FanoutMode:          policy.FanoutMode,
			FanoutPolicyVersion: policy.FanoutPolicyVersion,
			PermissionVersion:   member.PermissionVersion,
			Classification:      "MEMBER_BOUNDARY",
			MappingVersion:      string(eventType),
			TraceID:             traceID,
			PayloadJSON:         payloadJSON,
			CreatedAt:           occurredAt,
		})
		outbox = append(outbox, OutboxEvent{
			EventID:          eventID,
			TenantID:         command.AuthContext.TenantID,
			ConversationID:   command.ConversationID,
			AggregateVersion: seq,
			EventType:        eventType,
			EventVersion:     "v1",
			PartitionKey:     partitionKey,
			MappingVersion:   string(eventType),
			CorrelationID:    firstNonEmpty(command.AuthContext.RequestID, command.IdempotencyKey, string(eventID)),
			CausationID:      command.IdempotencyKey,
			Producer:         "conversation-service",
			PayloadJSON:      payloadJSON,
			TraceID:          traceID,
		})
	}
	return timeline, outbox, nil
}

func conversationCreateReason(conversationType types.ConversationType) string {
	if conversationType == types.ConversationTypeDirect {
		return "direct conversation created"
	}
	return "group created"
}
