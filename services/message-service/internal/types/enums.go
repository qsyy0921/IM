package types

type MessageType string

const (
	MessageTypeText MessageType = "TEXT"
)

type TimelineEventType string

const (
	TimelineEventMessagePersisted TimelineEventType = "message.persisted.v1"
	TimelineEventMessageEdited    TimelineEventType = "message.edited.v1"
	TimelineEventMessageRevoked   TimelineEventType = "message.revoked.v1"
	TimelineEventMessageDeleted   TimelineEventType = "message.deleted.v1"
)

type ConversationMode string

const (
	ConversationModeLocalRowLock  ConversationMode = "LOCAL_ROW_LOCK"
	ConversationModeSequencerBlock ConversationMode = "SEQUENCER_BLOCK"
)

type FanoutMode string

const (
	FanoutModeWriteFanout FanoutMode = "WRITE_FANOUT"
)
