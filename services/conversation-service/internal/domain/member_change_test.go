package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestNewMemberChangeRecordBuildsJoinedBoundary(t *testing.T) {
	now := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	record, err := NewMemberChangeRecord(MemberChangeInput{
		Command:      validMemberChangeCommand(),
		Conversation: activeConversation(),
		Target:       Member{UserID: "target-1"},
		Operator:     ownerMember("owner-1"),
	}, "change-1", "event-1", 11, now)
	if err != nil {
		t.Fatalf("new member change record: %v", err)
	}
	if record.Change.Status != types.MemberChangeStatusOutboxEnqueued ||
		record.Change.BoundarySeq != 11 ||
		record.Change.MemberVersion != 6 ||
		record.Change.PermissionVersion != 8 {
		t.Fatalf("unexpected change: %+v", record.Change)
	}
	if record.Target.NewStatus != types.MemberStatusActive ||
		record.Target.NewRole != types.MemberRoleMember ||
		record.Target.JoinSeq == nil ||
		*record.Target.JoinSeq != 11 {
		t.Fatalf("unexpected target mutation: %+v", record.Target)
	}
	if record.Timeline.EventType != types.TimelineEventConversationMemberJoined ||
		record.Timeline.EventID != "event-1" ||
		record.Timeline.ConversationSeq != 11 ||
		record.Timeline.ActorID != "owner-1" ||
		record.Timeline.PermissionVersion != 8 {
		t.Fatalf("unexpected timeline event: %+v", record.Timeline)
	}
	if record.Outbox.EventID != "event-1" ||
		record.Outbox.AggregateVersion != 11 ||
		record.Outbox.Producer != "conversation-service" ||
		record.Outbox.PartitionKey != "tenant-1:conv-1" {
		t.Fatalf("unexpected outbox event: %+v", record.Outbox)
	}
}

func TestNewMemberChangeRecordRejectsInvalidState(t *testing.T) {
	cases := []struct {
		name  string
		input MemberChangeInput
		err   error
	}{
		{
			name: "version conflict",
			input: func() MemberChangeInput {
				input := validMemberChangeInput()
				input.Command.ExpectedMemberVersion = 4
				return input
			}(),
			err: types.ErrMemberConflict,
		},
		{
			name: "sequencer mode not implemented",
			input: func() MemberChangeInput {
				input := validMemberChangeInput()
				input.Conversation.ConversationMode = types.ConversationModeSequencerBlock
				return input
			}(),
			err: types.ErrSequencerUnavailable,
		},
		{
			name: "member cannot add member",
			input: func() MemberChangeInput {
				input := validMemberChangeInput()
				input.Operator.Role = types.MemberRoleMember
				return input
			}(),
			err: types.ErrPermissionDenied,
		},
		{
			name: "join active member",
			input: func() MemberChangeInput {
				input := validMemberChangeInput()
				input.Target = memberWithRole("target-1", types.MemberRoleMember)
				return input
			}(),
			err: types.ErrMemberConflict,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewMemberChangeRecord(tc.input, "change-1", "event-1", 11, time.Now())
			if !errors.Is(err, tc.err) {
				t.Fatalf("expected %v, got %v", tc.err, err)
			}
		})
	}
}

func TestComputeMemberChangeCommandHashIsStable(t *testing.T) {
	command := validMemberChangeCommand()
	hash1, err := ComputeMemberChangeCommandHash(command)
	if err != nil {
		t.Fatalf("hash command: %v", err)
	}
	command.AuthContext.TraceID = "trace-2"
	command.AuthContext.RequestID = "request-2"
	hash2, err := ComputeMemberChangeCommandHash(command)
	if err != nil {
		t.Fatalf("hash command again: %v", err)
	}
	if hash1 != hash2 {
		t.Fatalf("trace/request id should not affect command hash: %s != %s", hash1, hash2)
	}
	command.TargetRole = types.MemberRoleAdmin
	hash3, err := ComputeMemberChangeCommandHash(command)
	if err != nil {
		t.Fatalf("hash changed command: %v", err)
	}
	if hash1 == hash3 {
		t.Fatal("target role change should affect command hash")
	}
}

func validMemberChangeInput() MemberChangeInput {
	return MemberChangeInput{
		Command:      validMemberChangeCommand(),
		Conversation: activeConversation(),
		Target:       Member{UserID: "target-1"},
		Operator:     ownerMember("owner-1"),
	}
}

func validMemberChangeCommand() types.CreateMemberChangeCommand {
	return types.CreateMemberChangeCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "owner-1",
			TraceID:   "trace-1",
			RequestID: "request-1",
		},
		ConversationID:        "conv-1",
		TargetUserID:          "target-1",
		ChangeType:            types.MemberChangeTypeJoin,
		TargetRole:            types.MemberRoleMember,
		ExpectedMemberVersion: 5,
		IdempotencyKey:        "idem-1",
		ConflictPolicy:        types.MemberChangeConflictPolicyReject,
		Reason:                "invite",
	}
}

func ownerMember(userID types.UserID) Member {
	member := memberWithRole(userID, types.MemberRoleOwner)
	member.MemberVersion = 5
	member.PermissionVersion = 7
	return member
}

func memberWithRole(userID types.UserID, role types.MemberRole) Member {
	return Member{
		UserID:            userID,
		Role:              role,
		Status:            types.MemberStatusActive,
		MemberVersion:     5,
		PermissionVersion: 7,
	}
}
