package types

import "time"

type PermissionDecision struct {
	Allowed           bool
	PermissionVersion int64
	Classification    string
	Reason            string
	OwnershipOverride bool
}

type ConversationSendContext struct {
	MemberVersion       int64
	PermissionVersion   int64
	ConversationMode    ConversationMode
	FanoutMode          FanoutMode
	FanoutPolicyVersion int64
	CurrentSeqShard     string
	DirectPeerUserID    UserID
}

type SeqBlock struct {
	StartSeq  int64
	EndSeq    int64
	Epoch     int64
	LeaseID   string
	ExpiresAt time.Time
}
