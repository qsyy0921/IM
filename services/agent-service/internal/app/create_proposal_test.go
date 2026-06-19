package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

func TestCreateAgentProposalUseCaseReturnsProposal(t *testing.T) {
	retrieval := &fakeRetrieval{result: types.RetrieveEvidenceResult{Pack: testEvidencePack()}}
	policy := &fakePolicy{decision: types.ToolPolicyDecision{
		TenantID:          "tenant-1",
		UserID:            "user-1",
		ToolName:          "conversation.note.create",
		Action:            types.ToolActionCall,
		ResourceType:      "conversation",
		ResourceID:        "conv-1",
		RiskLevel:         "LOW",
		Allowed:           true,
		RequiresApproval:  false,
		PermissionVersion: 3,
		Classification:    "TOOL_ALLOWED",
		Reason:            "allowed",
		DecisionSource:    "test",
	}}
	usecase := NewCreateAgentProposalUseCase(retrieval, policy)

	result, err := usecase.Execute(context.Background(), testCommand())
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	if result.Status != types.AgentProposalStatusProposed || result.ProposalID == "" ||
		!result.RequiresApproval || len(result.Citations) != 1 || len(result.EvidencePack.Items) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if retrieval.query.Query != "draft action plan" || retrieval.query.ConversationID != "conv-1" {
		t.Fatalf("unexpected retrieval query: %+v", retrieval.query)
	}
	if policy.command.ToolName != "conversation.note.create" || policy.command.Action != types.ToolActionCall {
		t.Fatalf("unexpected policy command: %+v", policy.command)
	}
}

func TestCreateAgentProposalUseCaseBlockedByPolicyDoesNotRetrieve(t *testing.T) {
	retrieval := &fakeRetrieval{err: errors.New("should not be called")}
	policy := &fakePolicy{decision: types.ToolPolicyDecision{
		Allowed:        false,
		Reason:         "tool policy denied",
		DecisionSource: "test",
	}}
	usecase := NewCreateAgentProposalUseCase(retrieval, policy)

	result, err := usecase.Execute(context.Background(), testCommand())
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	if result.Status != types.AgentProposalStatusBlocked || retrieval.called {
		t.Fatalf("expected blocked without retrieval, got result=%+v called=%v", result, retrieval.called)
	}
}

func TestCreateAgentProposalUseCaseInsufficientEvidence(t *testing.T) {
	retrieval := &fakeRetrieval{result: types.RetrieveEvidenceResult{Pack: types.EvidencePack{PackID: "pack-empty"}}}
	policy := &fakePolicy{decision: types.ToolPolicyDecision{Allowed: true}}
	usecase := NewCreateAgentProposalUseCase(retrieval, policy)

	result, err := usecase.Execute(context.Background(), testCommand())
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	if result.Status != types.AgentProposalStatusInsufficientEvidence || result.RequiresApproval {
		t.Fatalf("unexpected insufficient evidence result: %+v", result)
	}
}

func TestCreateAgentProposalUseCaseRejectsFabricatedCitation(t *testing.T) {
	retrieval := &fakeRetrieval{result: types.RetrieveEvidenceResult{Pack: testEvidencePack()}}
	policy := &fakePolicy{decision: types.ToolPolicyDecision{Allowed: true}}
	provider := fakeProvider{result: types.AgentProposalGenerationResult{
		ProposalText: "bad",
		Citations:    []types.Citation{{EvidenceID: "missing"}},
	}}
	usecase := NewCreateAgentProposalUseCaseWithProvider(retrieval, policy, provider)

	_, err := usecase.Execute(context.Background(), testCommand())
	if !errors.Is(err, types.ErrCitationVerification) {
		t.Fatalf("expected citation verification error, got %v", err)
	}
}

func testCommand() types.CreateAgentProposalCommand {
	return types.CreateAgentProposalCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conv-1",
		Objective:      "draft action plan",
		ToolName:       "conversation.note.create",
		ToolAction:     types.ToolActionCall,
		ResourceType:   "conversation",
		ResourceID:     "conv-1",
		RiskLevel:      "LOW",
		Intent:         "draft action plan",
		IncludeSearch:  true,
		IncludeMemory:  true,
	}
}

func testEvidencePack() types.EvidencePack {
	now := time.UnixMilli(1710000000000)
	return types.EvidencePack{
		PackID:         "pack-1",
		TenantID:       "tenant-1",
		ConversationID: "conv-1",
		Items: []types.EvidenceItem{{
			EvidenceID:      "evidence-1",
			SourceType:      types.EvidenceSourceSearchMessage,
			SourceID:        "message-1",
			ConversationID:  "conv-1",
			ConversationSeq: 7,
			Text:            "Alice asked Bob to draft the launch action plan.",
			OccurredAt:      now,
			SourceRefs: []types.EvidenceSourceRef{{
				SourceType:      types.EvidenceSourceSearchMessage,
				SourceID:        "message-1",
				SourceEventID:   "event-1",
				ConversationID:  "conv-1",
				ConversationSeq: 7,
				OccurredAt:      now,
			}},
		}},
	}
}

type fakeRetrieval struct {
	result types.RetrieveEvidenceResult
	err    error
	query  types.RetrieveEvidenceQuery
	called bool
}

func (fake *fakeRetrieval) RetrieveEvidence(
	_ context.Context,
	query types.RetrieveEvidenceQuery,
) (types.RetrieveEvidenceResult, error) {
	fake.called = true
	fake.query = query
	return fake.result, fake.err
}

type fakePolicy struct {
	decision types.ToolPolicyDecision
	err      error
	command  types.CheckToolActionCommand
}

func (fake *fakePolicy) CheckToolAction(
	_ context.Context,
	command types.CheckToolActionCommand,
) (types.ToolPolicyDecision, error) {
	fake.command = command
	return fake.decision, fake.err
}

type fakeProvider struct {
	result types.AgentProposalGenerationResult
	err    error
}

func (fake fakeProvider) GenerateProposal(
	context.Context,
	types.AgentProposalGenerationRequest,
) (types.AgentProposalGenerationResult, error) {
	return fake.result, fake.err
}
