package outbox

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func buildMemberBoundaryTimelineEvent(message types.OutboxMessage) (*conversationtimelinev1.ConversationTimelineEvent, error) {
	if message.EventType == types.TimelineEventConversationMemberOwnerTransferred {
		return buildOwnerTransferredTimelineEvent(message)
	}
	payload, err := decodeMemberBoundaryPayload(message.PayloadJSON)
	if err != nil {
		return nil, err
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, payload.OccurredAt)
	if err != nil {
		return nil, err
	}
	changeType, err := conversationMemberChangeType(payload.ChangeType)
	if err != nil {
		return nil, err
	}
	oldRole, err := conversationMemberRole(payload.OldRole)
	if err != nil {
		return nil, err
	}
	newRole, err := conversationMemberRole(payload.NewRole)
	if err != nil {
		return nil, err
	}
	oldStatus, err := conversationMemberStatus(payload.OldStatus)
	if err != nil {
		return nil, err
	}
	newStatus, err := conversationMemberStatus(payload.NewStatus)
	if err != nil {
		return nil, err
	}

	event := buildTimelineEnvelope(message, occurredAt)
	member := memberBoundaryProtoPayload{
		ChangeID:          payload.ChangeID,
		ConversationID:    payload.ConversationID,
		BoundarySeq:       payload.BoundarySeq,
		TargetUserID:      payload.TargetUserID,
		OperatorUserID:    payload.OperatorUserID,
		ChangeType:        changeType,
		OldRole:           oldRole,
		NewRole:           newRole,
		OldStatus:         oldStatus,
		NewStatus:         newStatus,
		MemberVersion:     payload.MemberVersion,
		PermissionVersion: payload.PermissionVersion,
		Reason:            payload.Reason,
		OccurredAt:        timestamppb.New(occurredAt),
	}
	switch message.EventType {
	case types.TimelineEventConversationMemberJoined:
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberJoined{
			ConversationMemberJoined: member.joined(),
		}
	case types.TimelineEventConversationMemberLeft:
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberLeft{
			ConversationMemberLeft: member.left(),
		}
	case types.TimelineEventConversationMemberRemoved:
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberRemoved{
			ConversationMemberRemoved: member.removed(),
		}
	case types.TimelineEventConversationMemberRoleChanged:
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberRoleChanged{
			ConversationMemberRoleChanged: member.roleChanged(),
		}
	case types.TimelineEventConversationMemberBoundaryCancelled:
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberBoundaryCancelled{
			ConversationMemberBoundaryCancelled: member.cancelled(),
		}
	case types.TimelineEventConversationMemberOwnerTransferred:
		return nil, errors.New("owner transfer must use owner transfer payload")
	default:
		return nil, errors.New("unsupported member boundary event type")
	}
	return event, nil
}

func buildOwnerTransferredTimelineEvent(message types.OutboxMessage) (*conversationtimelinev1.ConversationTimelineEvent, error) {
	payload, err := decodeOwnerTransferredPayload(message.PayloadJSON)
	if err != nil {
		return nil, err
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, payload.OccurredAt)
	if err != nil {
		return nil, err
	}
	changeType, err := conversationMemberChangeType(payload.ChangeType)
	if err != nil {
		return nil, err
	}
	previousOldRole, err := conversationMemberRole(payload.PreviousOwnerOldRole)
	if err != nil {
		return nil, err
	}
	previousNewRole, err := conversationMemberRole(payload.PreviousOwnerNewRole)
	if err != nil {
		return nil, err
	}
	newOwnerOldRole, err := conversationMemberRole(payload.NewOwnerOldRole)
	if err != nil {
		return nil, err
	}
	newOwnerNewRole, err := conversationMemberRole(payload.NewOwnerNewRole)
	if err != nil {
		return nil, err
	}
	previousStatus, err := conversationMemberStatus(payload.PreviousOwnerStatus)
	if err != nil {
		return nil, err
	}
	newOwnerStatus, err := conversationMemberStatus(payload.NewOwnerStatus)
	if err != nil {
		return nil, err
	}

	event := buildTimelineEnvelope(message, occurredAt)
	event.Payload = &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberOwnerTransferred{
		ConversationMemberOwnerTransferred: &conversationtimelinev1.ConversationMemberOwnerTransferredV1{
			ChangeId:             payload.ChangeID,
			ConversationId:       payload.ConversationID,
			BoundarySeq:          payload.BoundarySeq,
			PreviousOwnerUserId:  payload.PreviousOwnerUserID,
			NewOwnerUserId:       payload.NewOwnerUserID,
			OperatorUserId:       payload.OperatorUserID,
			ChangeType:           changeType,
			PreviousOwnerOldRole: previousOldRole,
			PreviousOwnerNewRole: previousNewRole,
			NewOwnerOldRole:      newOwnerOldRole,
			NewOwnerNewRole:      newOwnerNewRole,
			PreviousOwnerStatus:  previousStatus,
			NewOwnerStatus:       newOwnerStatus,
			MemberVersion:        payload.MemberVersion,
			PermissionVersion:    payload.PermissionVersion,
			Reason:               payload.Reason,
			OccurredAt:           timestamppb.New(occurredAt),
		},
	}
	return event, nil
}

type memberBoundaryPayload struct {
	ChangeID          string `json:"change_id"`
	ConversationID    string `json:"conversation_id"`
	BoundarySeq       int64  `json:"boundary_seq"`
	TargetUserID      string `json:"target_user_id"`
	OperatorUserID    string `json:"operator_user_id"`
	ChangeType        string `json:"change_type"`
	OldRole           string `json:"old_role"`
	NewRole           string `json:"new_role"`
	OldStatus         string `json:"old_status"`
	NewStatus         string `json:"new_status"`
	MemberVersion     int64  `json:"member_version"`
	PermissionVersion int64  `json:"permission_version"`
	Reason            string `json:"reason"`
	OccurredAt        string `json:"occurred_at"`
}

type ownerTransferredPayload struct {
	ChangeID             string `json:"change_id"`
	ConversationID       string `json:"conversation_id"`
	BoundarySeq          int64  `json:"boundary_seq"`
	PreviousOwnerUserID  string `json:"previous_owner_user_id"`
	NewOwnerUserID       string `json:"new_owner_user_id"`
	OperatorUserID       string `json:"operator_user_id"`
	ChangeType           string `json:"change_type"`
	PreviousOwnerOldRole string `json:"previous_owner_old_role"`
	PreviousOwnerNewRole string `json:"previous_owner_new_role"`
	NewOwnerOldRole      string `json:"new_owner_old_role"`
	NewOwnerNewRole      string `json:"new_owner_new_role"`
	PreviousOwnerStatus  string `json:"previous_owner_status"`
	NewOwnerStatus       string `json:"new_owner_status"`
	MemberVersion        int64  `json:"member_version"`
	PermissionVersion    int64  `json:"permission_version"`
	Reason               string `json:"reason"`
	OccurredAt           string `json:"occurred_at"`
}

func decodeMemberBoundaryPayload(payloadJSON []byte) (memberBoundaryPayload, error) {
	var payload memberBoundaryPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return memberBoundaryPayload{}, err
	}
	if payload.ChangeID == "" ||
		payload.ConversationID == "" ||
		payload.BoundarySeq <= 0 ||
		payload.TargetUserID == "" ||
		payload.OperatorUserID == "" ||
		payload.ChangeType == "" ||
		payload.MemberVersion <= 0 ||
		payload.PermissionVersion <= 0 ||
		payload.OccurredAt == "" {
		return memberBoundaryPayload{}, errors.New("member boundary payload is incomplete")
	}
	return payload, nil
}

func decodeOwnerTransferredPayload(payloadJSON []byte) (ownerTransferredPayload, error) {
	var payload ownerTransferredPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return ownerTransferredPayload{}, err
	}
	if payload.ChangeID == "" ||
		payload.ConversationID == "" ||
		payload.BoundarySeq <= 0 ||
		payload.PreviousOwnerUserID == "" ||
		payload.NewOwnerUserID == "" ||
		payload.OperatorUserID == "" ||
		payload.ChangeType == "" ||
		payload.PreviousOwnerOldRole == "" ||
		payload.PreviousOwnerNewRole == "" ||
		payload.NewOwnerOldRole == "" ||
		payload.NewOwnerNewRole == "" ||
		payload.PreviousOwnerStatus == "" ||
		payload.NewOwnerStatus == "" ||
		payload.MemberVersion <= 0 ||
		payload.PermissionVersion <= 0 ||
		payload.OccurredAt == "" {
		return ownerTransferredPayload{}, errors.New("owner transferred payload is incomplete")
	}
	if strings.ToUpper(payload.ChangeType) != "OWNER_TRANSFER" ||
		strings.ToUpper(payload.PreviousOwnerOldRole) != "OWNER" ||
		strings.ToUpper(payload.PreviousOwnerNewRole) != "ADMIN" ||
		strings.ToUpper(payload.NewOwnerNewRole) != "OWNER" ||
		strings.ToUpper(payload.PreviousOwnerStatus) != "ACTIVE" ||
		strings.ToUpper(payload.NewOwnerStatus) != "ACTIVE" {
		return ownerTransferredPayload{}, errors.New("owner transferred payload violates owner transfer contract")
	}
	switch strings.ToUpper(payload.NewOwnerOldRole) {
	case "ADMIN", "MEMBER":
	default:
		return ownerTransferredPayload{}, errors.New("owner transferred payload violates owner transfer contract")
	}
	return payload, nil
}

type memberBoundaryProtoPayload struct {
	ChangeID          string
	ConversationID    string
	BoundarySeq       int64
	TargetUserID      string
	OperatorUserID    string
	ChangeType        conversationtimelinev1.ConversationMemberChangeType
	OldRole           conversationtimelinev1.ConversationMemberRole
	NewRole           conversationtimelinev1.ConversationMemberRole
	OldStatus         conversationtimelinev1.ConversationMemberStatus
	NewStatus         conversationtimelinev1.ConversationMemberStatus
	MemberVersion     int64
	PermissionVersion int64
	Reason            string
	OccurredAt        *timestamppb.Timestamp
}

func (p memberBoundaryProtoPayload) joined() *conversationtimelinev1.ConversationMemberJoinedV1 {
	return &conversationtimelinev1.ConversationMemberJoinedV1{
		ChangeId:          p.ChangeID,
		ConversationId:    p.ConversationID,
		BoundarySeq:       p.BoundarySeq,
		TargetUserId:      p.TargetUserID,
		OperatorUserId:    p.OperatorUserID,
		ChangeType:        p.ChangeType,
		OldRole:           p.OldRole,
		NewRole:           p.NewRole,
		OldStatus:         p.OldStatus,
		NewStatus:         p.NewStatus,
		MemberVersion:     p.MemberVersion,
		PermissionVersion: p.PermissionVersion,
		Reason:            p.Reason,
		OccurredAt:        p.OccurredAt,
	}
}

func (p memberBoundaryProtoPayload) left() *conversationtimelinev1.ConversationMemberLeftV1 {
	return &conversationtimelinev1.ConversationMemberLeftV1{
		ChangeId:          p.ChangeID,
		ConversationId:    p.ConversationID,
		BoundarySeq:       p.BoundarySeq,
		TargetUserId:      p.TargetUserID,
		OperatorUserId:    p.OperatorUserID,
		ChangeType:        p.ChangeType,
		OldRole:           p.OldRole,
		NewRole:           p.NewRole,
		OldStatus:         p.OldStatus,
		NewStatus:         p.NewStatus,
		MemberVersion:     p.MemberVersion,
		PermissionVersion: p.PermissionVersion,
		Reason:            p.Reason,
		OccurredAt:        p.OccurredAt,
	}
}

func (p memberBoundaryProtoPayload) removed() *conversationtimelinev1.ConversationMemberRemovedV1 {
	return &conversationtimelinev1.ConversationMemberRemovedV1{
		ChangeId:          p.ChangeID,
		ConversationId:    p.ConversationID,
		BoundarySeq:       p.BoundarySeq,
		TargetUserId:      p.TargetUserID,
		OperatorUserId:    p.OperatorUserID,
		ChangeType:        p.ChangeType,
		OldRole:           p.OldRole,
		NewRole:           p.NewRole,
		OldStatus:         p.OldStatus,
		NewStatus:         p.NewStatus,
		MemberVersion:     p.MemberVersion,
		PermissionVersion: p.PermissionVersion,
		Reason:            p.Reason,
		OccurredAt:        p.OccurredAt,
	}
}

func (p memberBoundaryProtoPayload) roleChanged() *conversationtimelinev1.ConversationMemberRoleChangedV1 {
	return &conversationtimelinev1.ConversationMemberRoleChangedV1{
		ChangeId:          p.ChangeID,
		ConversationId:    p.ConversationID,
		BoundarySeq:       p.BoundarySeq,
		TargetUserId:      p.TargetUserID,
		OperatorUserId:    p.OperatorUserID,
		ChangeType:        p.ChangeType,
		OldRole:           p.OldRole,
		NewRole:           p.NewRole,
		OldStatus:         p.OldStatus,
		NewStatus:         p.NewStatus,
		MemberVersion:     p.MemberVersion,
		PermissionVersion: p.PermissionVersion,
		Reason:            p.Reason,
		OccurredAt:        p.OccurredAt,
	}
}

func (p memberBoundaryProtoPayload) cancelled() *conversationtimelinev1.ConversationMemberBoundaryCancelledV1 {
	return &conversationtimelinev1.ConversationMemberBoundaryCancelledV1{
		ChangeId:          p.ChangeID,
		ConversationId:    p.ConversationID,
		BoundarySeq:       p.BoundarySeq,
		TargetUserId:      p.TargetUserID,
		OperatorUserId:    p.OperatorUserID,
		ChangeType:        p.ChangeType,
		OldRole:           p.OldRole,
		NewRole:           p.NewRole,
		OldStatus:         p.OldStatus,
		NewStatus:         p.NewStatus,
		MemberVersion:     p.MemberVersion,
		PermissionVersion: p.PermissionVersion,
		Reason:            p.Reason,
		OccurredAt:        p.OccurredAt,
	}
}

func conversationMemberChangeType(value string) (conversationtimelinev1.ConversationMemberChangeType, error) {
	switch strings.ToUpper(value) {
	case "JOIN":
		return conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_JOIN, nil
	case "LEAVE":
		return conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_LEAVE, nil
	case "REMOVE":
		return conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_REMOVE, nil
	case "ROLE_CHANGED":
		return conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_ROLE_CHANGED, nil
	case "OWNER_TRANSFER":
		return conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_OWNER_TRANSFER, nil
	default:
		return conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_UNSPECIFIED, errors.New("unknown member change type")
	}
}

func conversationMemberRole(value string) (conversationtimelinev1.ConversationMemberRole, error) {
	switch strings.ToUpper(value) {
	case "":
		return conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_UNSPECIFIED, nil
	case "OWNER":
		return conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_OWNER, nil
	case "ADMIN":
		return conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_ADMIN, nil
	case "MEMBER":
		return conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER, nil
	default:
		return conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_UNSPECIFIED, errors.New("unknown member role")
	}
}

func conversationMemberStatus(value string) (conversationtimelinev1.ConversationMemberStatus, error) {
	switch strings.ToUpper(value) {
	case "":
		return conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_UNSPECIFIED, nil
	case "ACTIVE":
		return conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE, nil
	case "LEFT":
		return conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_LEFT, nil
	case "BANNED":
		return conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_BANNED, nil
	default:
		return conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_UNSPECIFIED, errors.New("unknown member status")
	}
}
