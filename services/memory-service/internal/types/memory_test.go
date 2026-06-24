package types

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestSubmitMemoryCandidateCommandValidatesFactHash(t *testing.T) {
	command := validSubmitMemoryCandidateCommand()
	command.FactSHA256 = strings.Repeat("0", 64)

	if err := command.Validate(); err != ErrInvalidArgument {
		t.Fatalf("Validate() error = %v, want invalid argument", err)
	}
}

func TestSubmitMemoryCandidateCommandRequiresSourceRefs(t *testing.T) {
	command := validSubmitMemoryCandidateCommand()
	command.SourceRefs = nil

	if err := command.Validate(); err != ErrInvalidArgument {
		t.Fatalf("Validate() error = %v, want invalid argument", err)
	}
}

func TestSubmitMemoryCandidateCommandAcceptsProfileSignalAsNeedsReviewCandidate(t *testing.T) {
	command := validSubmitMemoryCandidateCommand()
	command.EventType = MemoryEventTypeProfileSignal

	if err := command.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestSubmitMemoryCandidateCommandRejectsInvalidMemoryReferences(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*SubmitMemoryCandidateCommand)
	}{
		{
			name: "blank supersedes id",
			mutate: func(command *SubmitMemoryCandidateCommand) {
				command.SupersedesEventIDs = []string{" "}
			},
		},
		{
			name: "self supersedes id",
			mutate: func(command *SubmitMemoryCandidateCommand) {
				command.SupersedesEventIDs = []string{command.CandidateID}
			},
		},
		{
			name: "duplicate contradicts id",
			mutate: func(command *SubmitMemoryCandidateCommand) {
				command.ContradictsEventIDs = []string{"mem-1", "mem-1"}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			command := validSubmitMemoryCandidateCommand()
			tc.mutate(&command)
			if err := command.Validate(); err != ErrInvalidArgument {
				t.Fatalf("Validate() error = %v, want invalid argument", err)
			}
		})
	}
}

func validSubmitMemoryCandidateCommand() SubmitMemoryCandidateCommand {
	factText := "profile_signal: user prefers source backed memory"
	return SubmitMemoryCandidateCommand{
		AuthContext:       AuthContext{TenantID: "tenant-1", UserID: "user-1"},
		CandidateID:       "candidate-1",
		Scope:             MemoryScopeConversation,
		ScopeID:           "conv-1",
		ConversationID:    "conv-1",
		EventType:         MemoryEventTypeDecision,
		FactText:          factText,
		FactSHA256:        testFactSHA256(factText),
		ActorUserIDs:      []string{"user-1"},
		SourceRefs:        []SourceRef{{SourceType: MemorySourceTypeMessage, SourceID: "msg-1", SourceEventID: "event-1", ConversationID: "conv-1", ConversationSeq: 2}},
		ValidFromSeq:      2,
		Confidence:        0.8,
		ExtractionVersion: "memory-extraction-candidate-v1",
	}
}

func testFactSHA256(value string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
