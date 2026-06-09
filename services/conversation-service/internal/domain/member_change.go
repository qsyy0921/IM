package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type MemberChangeInput struct {
	Command      types.CreateMemberChangeCommand
	Conversation Conversation
	Target       Member
	Operator     Member
}

type MemberChangeRecord struct {
	Change   MemberChange
	Target   MemberMutation
	Timeline TimelineEvent
	Outbox   OutboxEvent
}

type MemberChange struct {
	ChangeID              types.ChangeID
	TenantID              types.TenantID
	ConversationID        types.ConversationID
	TargetUserID          types.UserID
	OperatorUserID        types.UserID
	ChangeType            types.MemberChangeType
	Status                types.MemberChangeStatus
	BoundarySeq           int64
	IdempotencyKey        string
	ExpectedMemberVersion int64
	CommandHash           string
	ConflictPolicy        types.MemberChangeConflictPolicy
	MemberVersion         int64
	PermissionVersion     int64
	TimelineEventID       types.EventID
	OutboxEventID         types.EventID
	MetadataJSON          []byte
	CreatedAt             time.Time
}

type MemberMutation struct {
	UserID            types.UserID
	OldRole           types.MemberRole
	NewRole           types.MemberRole
	OldStatus         types.MemberStatus
	NewStatus         types.MemberStatus
	MemberVersion     int64
	PermissionVersion int64
	JoinSeq           *int64
	LeaveSeq          *int64
}

type TimelineEvent struct {
	EventID             types.EventID
	EventType           types.TimelineEventType
	EventVersion        string
	TenantID            types.TenantID
	ConversationID      types.ConversationID
	ConversationSeq     int64
	ActorID             types.UserID
	FanoutMode          types.FanoutMode
	FanoutPolicyVersion int64
	PermissionVersion   int64
	Classification      string
	MappingVersion      string
	TraceID             string
	PayloadJSON         []byte
	CreatedAt           time.Time
}

type OutboxEvent struct {
	EventID          types.EventID
	TenantID         types.TenantID
	ConversationID   types.ConversationID
	AggregateVersion int64
	EventType        types.TimelineEventType
	EventVersion     string
	PartitionKey     string
	MappingVersion   string
	CorrelationID    string
	CausationID      string
	Producer         string
	PayloadJSON      []byte
	TraceID          string
}

func NewMemberChangeRecord(
	input MemberChangeInput,
	changeID types.ChangeID,
	eventID types.EventID,
	boundarySeq int64,
	now time.Time,
) (MemberChangeRecord, error) {
	if err := validateMemberChange(input); err != nil {
		return MemberChangeRecord{}, err
	}
	commandHash, err := ComputeMemberChangeCommandHash(input.Command)
	if err != nil {
		return MemberChangeRecord{}, err
	}
	mutation, err := buildMemberMutation(input, boundarySeq)
	if err != nil {
		return MemberChangeRecord{}, err
	}
	eventType := memberChangeEventType(input.Command.ChangeType)
	payloadJSON, err := buildMemberBoundaryPayload(input, changeID, boundarySeq, mutation, now)
	if err != nil {
		return MemberChangeRecord{}, err
	}
	metadataJSON, err := buildMemberChangeMetadata(mutation)
	if err != nil {
		return MemberChangeRecord{}, err
	}
	traceID := firstNonEmpty(input.Command.AuthContext.TraceID, input.Command.AuthContext.RequestID)
	partitionKey := string(input.Command.AuthContext.TenantID) + ":" + string(input.Command.ConversationID)

	return MemberChangeRecord{
		Change: MemberChange{
			ChangeID:              changeID,
			TenantID:              input.Command.AuthContext.TenantID,
			ConversationID:        input.Command.ConversationID,
			TargetUserID:          input.Command.TargetUserID,
			OperatorUserID:        input.Command.AuthContext.UserID,
			ChangeType:            input.Command.ChangeType,
			Status:                types.MemberChangeStatusOutboxEnqueued,
			BoundarySeq:           boundarySeq,
			IdempotencyKey:        input.Command.IdempotencyKey,
			ExpectedMemberVersion: input.Command.ExpectedMemberVersion,
			CommandHash:           commandHash,
			ConflictPolicy:        input.Command.ConflictPolicy,
			MemberVersion:         mutation.MemberVersion,
			PermissionVersion:     mutation.PermissionVersion,
			TimelineEventID:       eventID,
			OutboxEventID:         eventID,
			MetadataJSON:          metadataJSON,
			CreatedAt:             now.UTC(),
		},
		Target: mutation,
		Timeline: TimelineEvent{
			EventID:             eventID,
			EventType:           eventType,
			EventVersion:        "v1",
			TenantID:            input.Command.AuthContext.TenantID,
			ConversationID:      input.Command.ConversationID,
			ConversationSeq:     boundarySeq,
			ActorID:             input.Command.AuthContext.UserID,
			FanoutMode:          input.Conversation.FanoutMode,
			FanoutPolicyVersion: input.Conversation.FanoutPolicyVersion,
			PermissionVersion:   mutation.PermissionVersion,
			Classification:      "MEMBER_BOUNDARY",
			MappingVersion:      string(eventType),
			TraceID:             traceID,
			PayloadJSON:         payloadJSON,
			CreatedAt:           now.UTC(),
		},
		Outbox: OutboxEvent{
			EventID:          eventID,
			TenantID:         input.Command.AuthContext.TenantID,
			ConversationID:   input.Command.ConversationID,
			AggregateVersion: boundarySeq,
			EventType:        eventType,
			EventVersion:     "v1",
			PartitionKey:     partitionKey,
			MappingVersion:   string(eventType),
			CorrelationID:    firstNonEmpty(input.Command.AuthContext.RequestID, string(changeID)),
			CausationID:      string(changeID),
			Producer:         "conversation-service",
			PayloadJSON:      payloadJSON,
			TraceID:          traceID,
		},
	}, nil
}

func ComputeMemberChangeCommandHash(command types.CreateMemberChangeCommand) (string, error) {
	hashInput := struct {
		TenantID              types.TenantID                   `json:"tenant_id"`
		ConversationID        types.ConversationID             `json:"conversation_id"`
		TargetUserID          types.UserID                     `json:"target_user_id"`
		OperatorUserID        types.UserID                     `json:"operator_user_id"`
		ChangeType            types.MemberChangeType           `json:"change_type"`
		TargetRole            types.MemberRole                 `json:"target_role"`
		ExpectedMemberVersion int64                            `json:"expected_member_version"`
		IdempotencyKey        string                           `json:"idempotency_key"`
		ConflictPolicy        types.MemberChangeConflictPolicy `json:"conflict_policy"`
		Reason                string                           `json:"reason"`
	}{
		TenantID:              command.AuthContext.TenantID,
		ConversationID:        command.ConversationID,
		TargetUserID:          command.TargetUserID,
		OperatorUserID:        command.AuthContext.UserID,
		ChangeType:            command.ChangeType,
		TargetRole:            command.TargetRole,
		ExpectedMemberVersion: command.ExpectedMemberVersion,
		IdempotencyKey:        command.IdempotencyKey,
		ConflictPolicy:        command.ConflictPolicy,
		Reason:                command.Reason,
	}
	encoded, err := json.Marshal(hashInput)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func validateMemberChange(input MemberChangeInput) error {
	if input.Conversation.Status != types.ConversationStatusActive {
		return types.NewConversationNotFound("conversation is not active")
	}
	if input.Conversation.ConversationMode != types.ConversationModeLocalRowLock {
		return types.NewSequencerUnavailable("conversation mode is not implemented")
	}
	if input.Command.ExpectedMemberVersion > 0 &&
		input.Command.ExpectedMemberVersion != input.Conversation.MemberVersion {
		return types.NewMemberConflict("member version conflict")
	}
	if input.Command.ChangeType == types.MemberChangeTypeLeave &&
		input.Command.AuthContext.UserID == input.Command.TargetUserID {
		return nil
	}
	if input.Command.ChangeType == types.MemberChangeTypeLeave {
		return types.NewPermissionDenied("leave is only allowed for target user")
	}
	if input.Operator.Status != types.MemberStatusActive {
		return types.NewPermissionDenied("operator is not active")
	}
	switch input.Operator.Role {
	case types.MemberRoleOwner:
		return validateOwnerMemberChange(input)
	case types.MemberRoleAdmin:
		return validateAdminMemberChange(input)
	}
	return types.NewPermissionDenied("operator cannot change member")
}

func validateOwnerMemberChange(input MemberChangeInput) error {
	switch input.Command.ChangeType {
	case types.MemberChangeTypeJoin:
		if input.Command.TargetRole == types.MemberRoleOwner {
			return types.NewPermissionDenied("owner transfer is not supported")
		}
		return nil
	case types.MemberChangeTypeRemove:
		if input.Target.Role == types.MemberRoleOwner {
			return types.NewPermissionDenied("owner transfer is not supported")
		}
		return nil
	case types.MemberChangeTypeRoleChanged:
		if input.Target.Role == types.MemberRoleOwner || input.Command.TargetRole == types.MemberRoleOwner {
			return types.NewPermissionDenied("owner transfer is not supported")
		}
		return nil
	default:
		return types.NewPermissionDenied("operator cannot change member")
	}
}

func validateAdminMemberChange(input MemberChangeInput) error {
	switch input.Command.ChangeType {
	case types.MemberChangeTypeJoin:
		if input.Command.TargetRole != types.MemberRoleMember {
			return types.NewPermissionDenied("admin can only add member role")
		}
		return nil
	case types.MemberChangeTypeRemove:
		if input.Target.Role != types.MemberRoleMember {
			return types.NewPermissionDenied("admin can only remove ordinary member")
		}
		return nil
	default:
		return types.NewPermissionDenied("admin cannot perform member change")
	}
}

func buildMemberMutation(input MemberChangeInput, boundarySeq int64) (MemberMutation, error) {
	memberVersion := input.Conversation.MemberVersion + 1
	permissionVersion := input.Conversation.PermissionVersion + 1
	mutation := MemberMutation{
		UserID:            input.Command.TargetUserID,
		OldRole:           input.Target.Role,
		OldStatus:         input.Target.Status,
		NewRole:           input.Target.Role,
		NewStatus:         input.Target.Status,
		MemberVersion:     memberVersion,
		PermissionVersion: permissionVersion,
	}
	switch input.Command.ChangeType {
	case types.MemberChangeTypeJoin:
		if input.Target.Status == types.MemberStatusActive {
			return MemberMutation{}, types.NewMemberConflict("member already active")
		}
		mutation.NewRole = input.Command.TargetRole
		mutation.NewStatus = types.MemberStatusActive
		mutation.JoinSeq = &boundarySeq
	case types.MemberChangeTypeLeave:
		if input.Target.Status != types.MemberStatusActive {
			return MemberMutation{}, types.NewMemberConflict("member is not active")
		}
		mutation.NewStatus = types.MemberStatusLeft
		mutation.LeaveSeq = &boundarySeq
	case types.MemberChangeTypeRemove:
		if input.Target.Status != types.MemberStatusActive {
			return MemberMutation{}, types.NewMemberConflict("member is not active")
		}
		mutation.NewStatus = types.MemberStatusLeft
		mutation.LeaveSeq = &boundarySeq
	case types.MemberChangeTypeRoleChanged:
		if input.Target.Status != types.MemberStatusActive {
			return MemberMutation{}, types.NewMemberConflict("member is not active")
		}
		if input.Command.TargetRole == input.Target.Role {
			return MemberMutation{}, types.NewMemberConflict("member role unchanged")
		}
		mutation.NewRole = input.Command.TargetRole
	default:
		return MemberMutation{}, types.NewInvalidArgument("change_type is invalid")
	}
	if mutation.NewRole == "" {
		mutation.NewRole = types.MemberRoleMember
	}
	return mutation, nil
}

func memberChangeEventType(changeType types.MemberChangeType) types.TimelineEventType {
	switch changeType {
	case types.MemberChangeTypeJoin:
		return types.TimelineEventConversationMemberJoined
	case types.MemberChangeTypeLeave:
		return types.TimelineEventConversationMemberLeft
	case types.MemberChangeTypeRemove:
		return types.TimelineEventConversationMemberRemoved
	case types.MemberChangeTypeRoleChanged:
		return types.TimelineEventConversationMemberRoleChanged
	default:
		return types.TimelineEventConversationMemberBoundaryCancelled
	}
}

func buildMemberBoundaryPayload(
	input MemberChangeInput,
	changeID types.ChangeID,
	boundarySeq int64,
	mutation MemberMutation,
	occurredAt time.Time,
) ([]byte, error) {
	return json.Marshal(map[string]any{
		"change_id":          changeID,
		"conversation_id":    input.Command.ConversationID,
		"boundary_seq":       boundarySeq,
		"target_user_id":     input.Command.TargetUserID,
		"operator_user_id":   input.Command.AuthContext.UserID,
		"change_type":        input.Command.ChangeType,
		"old_role":           mutation.OldRole,
		"new_role":           mutation.NewRole,
		"old_status":         mutation.OldStatus,
		"new_status":         mutation.NewStatus,
		"member_version":     mutation.MemberVersion,
		"permission_version": mutation.PermissionVersion,
		"reason":             input.Command.Reason,
		"occurred_at":        occurredAt.UTC().Format(time.RFC3339Nano),
	})
}

func buildMemberChangeMetadata(mutation MemberMutation) ([]byte, error) {
	return json.Marshal(map[string]any{
		"old_role":           mutation.OldRole,
		"new_role":           mutation.NewRole,
		"old_status":         mutation.OldStatus,
		"new_status":         mutation.NewStatus,
		"member_version":     mutation.MemberVersion,
		"permission_version": mutation.PermissionVersion,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
