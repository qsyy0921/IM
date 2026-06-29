package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type OwnerTransferInput struct {
	Command       types.TransferConversationOwnerCommand
	Conversation  Conversation
	PreviousOwner Member
	NewOwner      Member
}

type OwnerTransferRecord struct {
	Change        MemberChange
	PreviousOwner MemberMutation
	NewOwner      MemberMutation
	Timeline      TimelineEvent
	Outbox        OutboxEvent
}

func NewOwnerTransferRecord(
	input OwnerTransferInput,
	changeID types.ChangeID,
	eventID types.EventID,
	boundarySeq int64,
	now time.Time,
) (OwnerTransferRecord, error) {
	if err := validateOwnerTransfer(input); err != nil {
		return OwnerTransferRecord{}, err
	}
	commandHash, err := ComputeOwnerTransferCommandHash(input.Command)
	if err != nil {
		return OwnerTransferRecord{}, err
	}
	previousOwner, newOwner := buildOwnerTransferMutations(input)
	payloadJSON, err := buildOwnerTransferPayload(input, changeID, boundarySeq, previousOwner, newOwner, now)
	if err != nil {
		return OwnerTransferRecord{}, err
	}
	metadataJSON, err := buildOwnerTransferMetadata(previousOwner, newOwner)
	if err != nil {
		return OwnerTransferRecord{}, err
	}
	traceID := firstNonEmpty(input.Command.AuthContext.TraceID, input.Command.AuthContext.RequestID)
	partitionKey := string(input.Command.AuthContext.TenantID) + ":" + string(input.Command.ConversationID)

	return OwnerTransferRecord{
		Change: MemberChange{
			ChangeID:              changeID,
			TenantID:              input.Command.AuthContext.TenantID,
			ConversationID:        input.Command.ConversationID,
			TargetUserID:          input.Command.NewOwnerUserID,
			OperatorUserID:        input.Command.AuthContext.UserID,
			ChangeType:            types.MemberChangeTypeOwnerTransfer,
			Status:                types.MemberChangeStatusOutboxEnqueued,
			BoundarySeq:           boundarySeq,
			IdempotencyKey:        input.Command.IdempotencyKey,
			ExpectedMemberVersion: input.Command.ExpectedMemberVersion,
			CommandHash:           commandHash,
			ConflictPolicy:        types.MemberChangeConflictPolicyReject,
			MemberVersion:         newOwner.MemberVersion,
			PermissionVersion:     newOwner.PermissionVersion,
			TimelineEventID:       eventID,
			OutboxEventID:         eventID,
			MetadataJSON:          metadataJSON,
			CreatedAt:             now.UTC(),
		},
		PreviousOwner: previousOwner,
		NewOwner:      newOwner,
		Timeline: TimelineEvent{
			EventID:             eventID,
			EventType:           types.TimelineEventConversationMemberOwnerTransferred,
			EventVersion:        "v1",
			TenantID:            input.Command.AuthContext.TenantID,
			ConversationID:      input.Command.ConversationID,
			ConversationSeq:     boundarySeq,
			ActorID:             input.Command.AuthContext.UserID,
			FanoutMode:          input.Conversation.FanoutMode,
			FanoutPolicyVersion: input.Conversation.FanoutPolicyVersion,
			PermissionVersion:   newOwner.PermissionVersion,
			Classification:      "MEMBER_BOUNDARY",
			MappingVersion:      string(types.TimelineEventConversationMemberOwnerTransferred),
			TraceID:             traceID,
			PayloadJSON:         payloadJSON,
			CreatedAt:           now.UTC(),
		},
		Outbox: OutboxEvent{
			EventID:          eventID,
			TenantID:         input.Command.AuthContext.TenantID,
			ConversationID:   input.Command.ConversationID,
			AggregateVersion: boundarySeq,
			EventType:        types.TimelineEventConversationMemberOwnerTransferred,
			EventVersion:     "v1",
			PartitionKey:     partitionKey,
			MappingVersion:   string(types.TimelineEventConversationMemberOwnerTransferred),
			CorrelationID:    firstNonEmpty(input.Command.AuthContext.RequestID, string(changeID)),
			CausationID:      string(changeID),
			Producer:         "conversation-service",
			PayloadJSON:      payloadJSON,
			TraceID:          traceID,
		},
	}, nil
}

func ComputeOwnerTransferCommandHash(command types.TransferConversationOwnerCommand) (string, error) {
	hashInput := struct {
		TenantID              types.TenantID       `json:"tenant_id"`
		ConversationID        types.ConversationID `json:"conversation_id"`
		PreviousOwnerUserID   types.UserID         `json:"previous_owner_user_id"`
		NewOwnerUserID        types.UserID         `json:"new_owner_user_id"`
		ExpectedMemberVersion int64                `json:"expected_member_version"`
		IdempotencyKey        string               `json:"idempotency_key"`
		Reason                string               `json:"reason"`
	}{
		TenantID:              command.AuthContext.TenantID,
		ConversationID:        command.ConversationID,
		PreviousOwnerUserID:   command.AuthContext.UserID,
		NewOwnerUserID:        command.NewOwnerUserID,
		ExpectedMemberVersion: command.ExpectedMemberVersion,
		IdempotencyKey:        command.IdempotencyKey,
		Reason:                command.Reason,
	}
	encoded, err := json.Marshal(hashInput)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func validateOwnerTransfer(input OwnerTransferInput) error {
	if input.Conversation.Status != types.ConversationStatusActive {
		return types.NewConversationNotFound("conversation is not active")
	}
	switch input.Conversation.ConversationMode {
	case types.ConversationModeLocalRowLock, types.ConversationModeSequencerBlock:
	default:
		return types.NewSequencerUnavailable("conversation mode is not supported")
	}
	if input.Command.ExpectedMemberVersion > 0 &&
		input.Command.ExpectedMemberVersion != input.Conversation.MemberVersion {
		return types.NewMemberConflict("member version conflict")
	}
	if input.PreviousOwner.UserID != input.Command.AuthContext.UserID ||
		input.PreviousOwner.Status != types.MemberStatusActive ||
		input.PreviousOwner.Role != types.MemberRoleOwner {
		return types.NewPermissionDenied("operator must be active owner")
	}
	if input.NewOwner.UserID != input.Command.NewOwnerUserID ||
		input.NewOwner.Status != types.MemberStatusActive {
		return types.NewMemberConflict("new owner must be active member")
	}
	switch input.NewOwner.Role {
	case types.MemberRoleAdmin, types.MemberRoleMember:
		return nil
	default:
		return types.NewPermissionDenied("new owner must be admin or member")
	}
}

func buildOwnerTransferMutations(input OwnerTransferInput) (MemberMutation, MemberMutation) {
	memberVersion := input.Conversation.MemberVersion + 1
	permissionVersion := input.Conversation.PermissionVersion + 1
	previousOwner := MemberMutation{
		UserID:            input.Command.AuthContext.UserID,
		OldRole:           input.PreviousOwner.Role,
		NewRole:           types.MemberRoleAdmin,
		OldStatus:         input.PreviousOwner.Status,
		NewStatus:         types.MemberStatusActive,
		MemberVersion:     memberVersion,
		PermissionVersion: permissionVersion,
	}
	newOwner := MemberMutation{
		UserID:            input.Command.NewOwnerUserID,
		OldRole:           input.NewOwner.Role,
		NewRole:           types.MemberRoleOwner,
		OldStatus:         input.NewOwner.Status,
		NewStatus:         types.MemberStatusActive,
		MemberVersion:     memberVersion,
		PermissionVersion: permissionVersion,
	}
	return previousOwner, newOwner
}

func buildOwnerTransferPayload(
	input OwnerTransferInput,
	changeID types.ChangeID,
	boundarySeq int64,
	previousOwner MemberMutation,
	newOwner MemberMutation,
	occurredAt time.Time,
) ([]byte, error) {
	return json.Marshal(map[string]any{
		"change_id":               changeID,
		"conversation_id":         input.Command.ConversationID,
		"boundary_seq":            boundarySeq,
		"previous_owner_user_id":  input.Command.AuthContext.UserID,
		"new_owner_user_id":       input.Command.NewOwnerUserID,
		"operator_user_id":        input.Command.AuthContext.UserID,
		"change_type":             types.MemberChangeTypeOwnerTransfer,
		"previous_owner_old_role": previousOwner.OldRole,
		"previous_owner_new_role": previousOwner.NewRole,
		"new_owner_old_role":      newOwner.OldRole,
		"new_owner_new_role":      newOwner.NewRole,
		"previous_owner_status":   previousOwner.NewStatus,
		"new_owner_status":        newOwner.NewStatus,
		"member_version":          newOwner.MemberVersion,
		"permission_version":      newOwner.PermissionVersion,
		"reason":                  input.Command.Reason,
		"occurred_at":             occurredAt.UTC().Format(time.RFC3339Nano),
	})
}

func buildOwnerTransferMetadata(previousOwner MemberMutation, newOwner MemberMutation) ([]byte, error) {
	return json.Marshal(map[string]any{
		"previous_owner_old_role": previousOwner.OldRole,
		"previous_owner_new_role": previousOwner.NewRole,
		"previous_owner_status":   previousOwner.NewStatus,
		"new_owner_old_role":      newOwner.OldRole,
		"new_owner_new_role":      newOwner.NewRole,
		"new_owner_status":        newOwner.NewStatus,
		"member_version":          newOwner.MemberVersion,
		"permission_version":      newOwner.PermissionVersion,
	})
}
