package types

import "time"

type AllocateSeqBlockCommand struct {
	TenantID        string
	ConversationID  string
	RequesterID     string
	BlockSize       int
	IdempotencyKey  string
	MinimumStartSeq int64
}

func (command AllocateSeqBlockCommand) Validate(maxBlockSize int) error {
	if command.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if command.RequesterID == "" {
		return NewInvalidArgument("requester_id is required")
	}
	if command.BlockSize <= 0 {
		return NewInvalidArgument("block_size must be positive")
	}
	if maxBlockSize > 0 && command.BlockSize > maxBlockSize {
		return NewInvalidArgument("block_size exceeds max")
	}
	if command.IdempotencyKey == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if command.MinimumStartSeq < 0 {
		return NewInvalidArgument("minimum_start_seq must be non-negative")
	}
	return nil
}

type SeqBlockLease struct {
	TenantID         string
	ConversationID   string
	StartSeq         int64
	EndSeq           int64
	BlockSize        int
	SequencerEpoch   int64
	LeaseID          string
	ExpiresAt        time.Time
	IdempotentReplay bool
}
