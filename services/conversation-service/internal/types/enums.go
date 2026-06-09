package types

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

type ConversationStatus string

const (
	ConversationStatusActive   ConversationStatus = "ACTIVE"
	ConversationStatusArchived ConversationStatus = "ARCHIVED"
	ConversationStatusDeleted  ConversationStatus = "DELETED"
)

type MemberStatus string

const (
	MemberStatusActive MemberStatus = "ACTIVE"
	MemberStatusLeft   MemberStatus = "LEFT"
	MemberStatusBanned MemberStatus = "BANNED"
)
