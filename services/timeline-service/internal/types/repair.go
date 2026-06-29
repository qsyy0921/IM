package types

import (
	"strings"
	"time"
)

type ExpireLeasesCommand struct {
	Before     time.Time
	Limit      int
	DryRun     bool
	OperatorID string
	Reason     string
}

func (command ExpireLeasesCommand) Validate() error {
	if command.Before.IsZero() {
		return NewInvalidArgument("before is required")
	}
	if command.Limit <= 0 {
		return NewInvalidArgument("limit must be positive")
	}
	if strings.TrimSpace(command.OperatorID) == "" {
		return NewInvalidArgument("operator_id is required")
	}
	if strings.TrimSpace(command.Reason) == "" {
		return NewInvalidArgument("reason is required")
	}
	return nil
}

type ExpireLeasesResult struct {
	Matched int
	Expired int
	DryRun  bool
}

type GapMarkerCommand struct {
	TenantID       string
	ConversationID string
	StartSeq       int64
	EndSeq         int64
	SequencerEpoch int64
	LeaseID        string
	Reason         string
	OperatorID     string
	DryRun         bool
}

func (command GapMarkerCommand) Validate() error {
	if strings.TrimSpace(command.TenantID) == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if strings.TrimSpace(command.ConversationID) == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if command.StartSeq <= 0 || command.EndSeq < command.StartSeq {
		return NewInvalidArgument("invalid gap range")
	}
	if command.SequencerEpoch <= 0 {
		return NewInvalidArgument("sequencer_epoch is required")
	}
	if strings.TrimSpace(command.LeaseID) == "" {
		return NewInvalidArgument("lease_id is required")
	}
	if strings.TrimSpace(command.Reason) == "" {
		return NewInvalidArgument("reason is required")
	}
	if strings.TrimSpace(command.OperatorID) == "" {
		return NewInvalidArgument("operator_id is required")
	}
	return nil
}

type GapMarker struct {
	MarkerID       string
	TenantID       string
	ConversationID string
	StartSeq       int64
	EndSeq         int64
	SequencerEpoch int64
	LeaseID        string
	Reason         string
	Status         string
	CreatedBy      string
	CreatedAt      time.Time
	ClosedBy       string
	ClosedAt       *time.Time
	CloseReason    string
}

type CloseGapMarkerCommand struct {
	MarkerID    string
	OperatorID  string
	CloseReason string
	DryRun      bool
}

func (command CloseGapMarkerCommand) Validate() error {
	if strings.TrimSpace(command.MarkerID) == "" {
		return NewInvalidArgument("marker_id is required")
	}
	if strings.TrimSpace(command.OperatorID) == "" {
		return NewInvalidArgument("operator_id is required")
	}
	if strings.TrimSpace(command.CloseReason) == "" {
		return NewInvalidArgument("close_reason is required")
	}
	return nil
}
