package types

type MessageType string

const (
	MessageTypeText     MessageType = "TEXT"
	MessageTypeImage    MessageType = "IMAGE"
	MessageTypeFile     MessageType = "FILE"
	MessageTypeVoice    MessageType = "VOICE"
	MessageTypeLocation MessageType = "LOCATION"
	MessageTypeCard     MessageType = "CARD"
)

func IsSupportedMessageType(messageType MessageType) bool {
	switch messageType {
	case MessageTypeText,
		MessageTypeImage,
		MessageTypeFile,
		MessageTypeVoice,
		MessageTypeLocation,
		MessageTypeCard:
		return true
	default:
		return false
	}
}

func MessageTypeRequiresAttachment(messageType MessageType) bool {
	switch messageType {
	case MessageTypeImage, MessageTypeFile, MessageTypeVoice:
		return true
	default:
		return false
	}
}

type TimelineEventType string

const (
	TimelineEventMessagePersisted TimelineEventType = "message.persisted.v1"
	TimelineEventMessageEdited    TimelineEventType = "message.edited.v1"
	TimelineEventMessageRevoked   TimelineEventType = "message.revoked.v1"
	TimelineEventMessageDeleted   TimelineEventType = "message.deleted.v1"

	TimelineEventConversationMemberJoined            TimelineEventType = "conversation.member.joined.v1"
	TimelineEventConversationMemberLeft              TimelineEventType = "conversation.member.left.v1"
	TimelineEventConversationMemberRemoved           TimelineEventType = "conversation.member.removed.v1"
	TimelineEventConversationMemberRoleChanged       TimelineEventType = "conversation.member.role_changed.v1"
	TimelineEventConversationMemberBoundaryCancelled TimelineEventType = "conversation.member.boundary_cancelled.v1"
	TimelineEventConversationMemberOwnerTransferred  TimelineEventType = "conversation.member.owner_transferred.v1"
)

type ConversationMode string

const (
	ConversationModeLocalRowLock   ConversationMode = "LOCAL_ROW_LOCK"
	ConversationModeSequencerBlock ConversationMode = "SEQUENCER_BLOCK"
)

type FanoutMode string

const (
	FanoutModeWriteFanout     FanoutMode = "WRITE_FANOUT"
	FanoutModeHybridFanout    FanoutMode = "HYBRID_FANOUT"
	FanoutModeReadFanout      FanoutMode = "READ_FANOUT"
	FanoutModeBroadcastSignal FanoutMode = "BROADCAST_SIGNAL"
)
