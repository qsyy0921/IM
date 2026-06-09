package domain

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestNewOwnerTransferRecordBuildsPayloadAndMetadata(t *testing.T) {
	now := time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)

	record, err := NewOwnerTransferRecord(validOwnerTransferInput(), "change-owner-1", "event-owner-1", 12, now)
	if err != nil {
		t.Fatalf("new owner transfer record: %v", err)
	}
	if record.Change.ChangeType != types.MemberChangeTypeOwnerTransfer ||
		record.Change.TargetUserID != "user-2" ||
		record.Change.OperatorUserID != "owner-1" ||
		record.Change.BoundarySeq != 12 ||
		record.Change.MemberVersion != 8 ||
		record.Change.PermissionVersion != 10 {
		t.Fatalf("unexpected change: %+v", record.Change)
	}
	if record.PreviousOwner.NewRole != types.MemberRoleAdmin ||
		record.NewOwner.NewRole != types.MemberRoleOwner ||
		record.Timeline.EventType != types.TimelineEventConversationMemberOwnerTransferred ||
		record.Outbox.EventType != types.TimelineEventConversationMemberOwnerTransferred {
		t.Fatalf("unexpected record: %+v", record)
	}

	var payload map[string]any
	if err := json.Unmarshal(record.Outbox.PayloadJSON, &payload); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	if payload["previous_owner_user_id"] != "owner-1" ||
		payload["new_owner_user_id"] != "user-2" ||
		payload["previous_owner_old_role"] != "OWNER" ||
		payload["previous_owner_new_role"] != "ADMIN" ||
		payload["new_owner_old_role"] != "MEMBER" ||
		payload["new_owner_new_role"] != "OWNER" ||
		payload["previous_owner_status"] != "ACTIVE" ||
		payload["new_owner_status"] != "ACTIVE" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestNewOwnerTransferRecordRejectsInvalidRules(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*OwnerTransferInput)
		wantErr error
	}{
		{
			name: "operator not owner",
			mutate: func(input *OwnerTransferInput) {
				input.PreviousOwner.Role = types.MemberRoleAdmin
			},
			wantErr: types.ErrPermissionDenied,
		},
		{
			name: "new owner inactive",
			mutate: func(input *OwnerTransferInput) {
				input.NewOwner.Status = types.MemberStatusLeft
			},
			wantErr: types.ErrMemberConflict,
		},
		{
			name: "new owner already owner",
			mutate: func(input *OwnerTransferInput) {
				input.NewOwner.Role = types.MemberRoleOwner
			},
			wantErr: types.ErrPermissionDenied,
		},
		{
			name: "member version conflict",
			mutate: func(input *OwnerTransferInput) {
				input.Command.ExpectedMemberVersion = 99
			},
			wantErr: types.ErrMemberConflict,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := validOwnerTransferInput()
			tc.mutate(&input)

			_, err := NewOwnerTransferRecord(input, "change-owner-1", "event-owner-1", 12, time.Now())
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func validOwnerTransferInput() OwnerTransferInput {
	return OwnerTransferInput{
		Command: types.TransferConversationOwnerCommand{
			AuthContext: types.AuthContext{
				TenantID:  "tenant-1",
				UserID:    "owner-1",
				RequestID: "request-1",
				TraceID:   "trace-1",
			},
			ConversationID:        "conv-1",
			NewOwnerUserID:        "user-2",
			ExpectedMemberVersion: 7,
			IdempotencyKey:        "transfer-owner-1",
			Reason:                "handoff",
		},
		Conversation: Conversation{
			TenantID:            "tenant-1",
			ConversationID:      "conv-1",
			Status:              types.ConversationStatusActive,
			ConversationMode:    types.ConversationModeLocalRowLock,
			FanoutMode:          types.FanoutModeWriteFanout,
			FanoutPolicyVersion: 3,
			MemberVersion:       7,
			PermissionVersion:   9,
			CurrentSeqShard:     "local",
		},
		PreviousOwner: Member{
			UserID:            "owner-1",
			Role:              types.MemberRoleOwner,
			Status:            types.MemberStatusActive,
			MemberVersion:     7,
			PermissionVersion: 9,
		},
		NewOwner: Member{
			UserID:            "user-2",
			Role:              types.MemberRoleMember,
			Status:            types.MemberStatusActive,
			MemberVersion:     7,
			PermissionVersion: 9,
		},
	}
}
