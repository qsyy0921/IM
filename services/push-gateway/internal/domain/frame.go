package domain

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

func NewOpaqueID(prefix string) string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + "_" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}

func ServerHello(requestID string, result types.ConnectSessionResult) types.ServerFrame {
	return types.ServerFrame{
		Op:                types.OpServerHello,
		RequestID:         requestID,
		SessionID:         result.SessionID,
		ResumeToken:       result.ResumeToken,
		HeartbeatInterval: result.HeartbeatIntervalMS,
	}
}

func ServerPong(requestID string, now time.Time) types.ServerFrame {
	return types.ServerFrame{
		Op:           types.OpServerPong,
		RequestID:    requestID,
		ServerTimeMS: now.UnixMilli(),
	}
}

func DeliveryNotify(notification types.DeliveryNotification) types.ServerFrame {
	if notification.Kind == types.DeliveryNotificationKindInboxItemHidden {
		return DeliveryHide(notification)
	}
	return types.ServerFrame{
		Op:              types.OpDeliveryNotify,
		EventID:         notification.EventID,
		TenantID:        notification.TenantID,
		ConversationID:  notification.ConversationID,
		ConversationSeq: notification.ConversationSeq,
		SourceEventID:   notification.SourceEventID,
		SourceEventType: notification.SourceEventType,
		MessageID:       notification.MessageID,
		CorrelationID:   notification.CorrelationID,
		PullRequired:    true,
	}
}

func DeliveryHide(notification types.DeliveryNotification) types.ServerFrame {
	return types.ServerFrame{
		Op:              types.OpDeliveryHide,
		EventID:         notification.EventID,
		TenantID:        notification.TenantID,
		ConversationID:  notification.ConversationID,
		ConversationSeq: notification.ConversationSeq,
		SourceEventID:   notification.SourceEventID,
		SourceEventType: notification.SourceEventType,
		MessageID:       notification.MessageID,
		CorrelationID:   notification.CorrelationID,
		PullRequired:    true,
	}
}

func DeliveryAckOK(requestID string, result types.AckDeliveryResult) types.ServerFrame {
	return types.ServerFrame{
		Op:              types.OpDeliveryAckOK,
		RequestID:       requestID,
		ConversationID:  result.ConversationID,
		LastReceivedSeq: result.LastReceivedSeq,
	}
}

func ConversationSubscribeOK(requestID string, result types.ConversationSubscriptionResult) types.ServerFrame {
	return types.ServerFrame{
		Op:             types.OpConversationSubscribeOK,
		RequestID:      requestID,
		ConversationID: result.ConversationID,
	}
}

func ConversationUnsubscribeOK(requestID string, result types.ConversationSubscriptionResult) types.ServerFrame {
	return types.ServerFrame{
		Op:             types.OpConversationUnsubscribeOK,
		RequestID:      requestID,
		ConversationID: result.ConversationID,
	}
}

func ResumeHint(reason string, conversations []types.ConversationCursor) types.ServerFrame {
	return types.ServerFrame{
		Op:            types.OpResumeHint,
		Reason:        reason,
		PullRequired:  true,
		Conversations: conversations,
	}
}

func ErrorFrame(requestID string, code string, message string, retryable bool) types.ServerFrame {
	return types.ServerFrame{
		Op:        types.OpError,
		RequestID: requestID,
		Code:      code,
		Message:   message,
		Retryable: retryable,
	}
}
