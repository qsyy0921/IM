package types

type ConversationMode string

type ConversationType string

const (
	ConversationTypeDirect ConversationType = "DIRECT"
	ConversationTypeGroup  ConversationType = "GROUP"
)

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

type MemberRole string

const (
	MemberRoleOwner  MemberRole = "OWNER"
	MemberRoleAdmin  MemberRole = "ADMIN"
	MemberRoleMember MemberRole = "MEMBER"
)

type MemberChangeType string

const (
	MemberChangeTypeJoin          MemberChangeType = "JOIN"
	MemberChangeTypeLeave         MemberChangeType = "LEAVE"
	MemberChangeTypeRemove        MemberChangeType = "REMOVE"
	MemberChangeTypeRoleChanged   MemberChangeType = "ROLE_CHANGED"
	MemberChangeTypeOwnerTransfer MemberChangeType = "OWNER_TRANSFER"
)

type MemberChangeConflictPolicy string

const (
	MemberChangeConflictPolicyReject     MemberChangeConflictPolicy = "REJECT"
	MemberChangeConflictPolicyMerge      MemberChangeConflictPolicy = "MERGE"
	MemberChangeConflictPolicyCompensate MemberChangeConflictPolicy = "COMPENSATE"
)

type MemberChangeStatus string

const (
	MemberChangeStatusPendingBoundary   MemberChangeStatus = "PENDING_BOUNDARY"
	MemberChangeStatusBoundaryAllocated MemberChangeStatus = "BOUNDARY_ALLOCATED"
	MemberChangeStatusMemberUpdated     MemberChangeStatus = "MEMBER_UPDATED"
	MemberChangeStatusOutboxEnqueued    MemberChangeStatus = "OUTBOX_ENQUEUED"
	MemberChangeStatusEventPublished    MemberChangeStatus = "EVENT_PUBLISHED"
	MemberChangeStatusDone              MemberChangeStatus = "DONE"
	MemberChangeStatusFailedCompensated MemberChangeStatus = "FAILED_COMPENSATED"
)

type TimelineEventType string

const (
	TimelineEventConversationMemberJoined            TimelineEventType = "conversation.member.joined.v1"
	TimelineEventConversationMemberLeft              TimelineEventType = "conversation.member.left.v1"
	TimelineEventConversationMemberRemoved           TimelineEventType = "conversation.member.removed.v1"
	TimelineEventConversationMemberRoleChanged       TimelineEventType = "conversation.member.role_changed.v1"
	TimelineEventConversationMemberBoundaryCancelled TimelineEventType = "conversation.member.boundary_cancelled.v1"
	TimelineEventConversationMemberOwnerTransferred  TimelineEventType = "conversation.member.owner_transferred.v1"
)
