package types

type ConversationSendContext struct {
	TenantID            TenantID
	ConversationID      ConversationID
	MemberVersion       int64
	PermissionVersion   int64
	ConversationMode    ConversationMode
	FanoutMode          FanoutMode
	FanoutPolicyVersion int64
	CurrentSeqShard     string
	DirectPeerUserID    UserID
}
