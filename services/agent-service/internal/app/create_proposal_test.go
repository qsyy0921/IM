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
	prepare := &fakePrepare{result: types.ToolPrepareResult{
		TenantID:          "tenant-1",
		UserID:            "user-1",
		SkillID:           "conversation.note.create",
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
		AuditID:           "mcp-audit-1",
	}}
	usecase := NewCreateAgentProposalUseCase(retrieval, prepare)

	result, err := usecase.Execute(context.Background(), testCommand())
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	if result.Status != types.AgentProposalStatusProposed || result.ProposalID == "" ||
		!result.RequiresApproval || result.PreparedAuditID != "mcp-audit-1" ||
		result.SkillID != "conversation.note.create" || len(result.Citations) != 1 || len(result.EvidencePack.Items) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if retrieval.query.Query != "draft action plan" || retrieval.query.ConversationID != "conv-1" {
		t.Fatalf("unexpected retrieval query: %+v", retrieval.query)
	}
	if prepare.command.SkillID != "conversation.note.create" ||
		prepare.command.ToolName != "conversation.note.create" ||
		prepare.command.Action != types.ToolActionCall ||
		prepare.command.IdempotencyKey == "" ||
		prepare.command.InputJSON == "" {
		t.Fatalf("unexpected prepare command: %+v", prepare.command)
	}
}

func TestCreateAgentProposalUseCaseStoresProposalWhenRepositoryConfigured(t *testing.T) {
	store := &fakeProposalStore{}
	retrieval := &fakeRetrieval{result: types.RetrieveEvidenceResult{Pack: testEvidencePack()}}
	prepare := &fakePrepare{result: types.ToolPrepareResult{
		TenantID:          "tenant-1",
		UserID:            "user-1",
		SkillID:           "conversation.note.create",
		ToolName:          "conversation.note.create",
		Action:            types.ToolActionCall,
		ResourceType:      "conversation",
		ResourceID:        "conv-1",
		RiskLevel:         "LOW",
		Allowed:           true,
		RequiresApproval:  true,
		PermissionVersion: 3,
		Classification:    "TOOL_ALLOWED",
		Reason:            "allowed",
		DecisionSource:    "test",
		AuditID:           "mcp-audit-1",
	}}
	usecase := NewCreateAgentProposalUseCaseWithRepository(retrieval, prepare, ExtractiveProposalProvider{}, store)

	result, err := usecase.Execute(context.Background(), testCommand())
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	if len(store.proposals) != 1 {
		t.Fatalf("expected one stored proposal, got %d", len(store.proposals))
	}
	stored := store.proposals[0]
	if stored.ProposalID != result.ProposalID ||
		stored.Status != types.AgentProposalStatusProposed ||
		stored.PreparedAuditID != "mcp-audit-1" ||
		stored.CitationsJSON == "" ||
		stored.EvidencePackID != "pack-1" {
		t.Fatalf("unexpected stored proposal: %+v", stored)
	}
}

func TestCreateAgentProposalUseCaseBlockedByPolicyDoesNotRetrieve(t *testing.T) {
	retrieval := &fakeRetrieval{err: errors.New("should not be called")}
	prepare := &fakePrepare{result: types.ToolPrepareResult{
		SkillID:        "conversation.note.create",
		Allowed:        false,
		Reason:         "tool policy denied",
		DecisionSource: "test",
		AuditID:        "mcp-audit-deny",
	}}
	usecase := NewCreateAgentProposalUseCase(retrieval, prepare)

	result, err := usecase.Execute(context.Background(), testCommand())
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	if result.Status != types.AgentProposalStatusBlocked || result.PreparedAuditID != "mcp-audit-deny" || retrieval.called {
		t.Fatalf("expected blocked without retrieval, got result=%+v called=%v", result, retrieval.called)
	}
}

func TestCreateAgentProposalUseCaseInsufficientEvidence(t *testing.T) {
	retrieval := &fakeRetrieval{result: types.RetrieveEvidenceResult{Pack: types.EvidencePack{PackID: "pack-empty"}}}
	prepare := &fakePrepare{result: types.ToolPrepareResult{Allowed: true, SkillID: "conversation.note.create", AuditID: "mcp-audit-1"}}
	usecase := NewCreateAgentProposalUseCase(retrieval, prepare)

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
	prepare := &fakePrepare{result: types.ToolPrepareResult{Allowed: true, SkillID: "conversation.note.create"}}
	provider := fakeProvider{result: types.AgentProposalGenerationResult{
		ProposalText: "bad",
		Citations:    []types.Citation{{EvidenceID: "missing"}},
	}}
	usecase := NewCreateAgentProposalUseCaseWithProvider(retrieval, prepare, provider)

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

type fakePrepare struct {
	result  types.ToolPrepareResult
	err     error
	command types.PrepareToolCallCommand
}

func (fake *fakePrepare) PrepareToolCall(
	_ context.Context,
	command types.PrepareToolCallCommand,
) (types.ToolPrepareResult, error) {
	fake.command = command
	return fake.result, fake.err
}

type fakeProvider struct {
	result types.AgentProposalGenerationResult
	err    error
}

type fakeProposalStore struct {
	proposals []types.StoredAgentProposal
	err       error
}

func (store *fakeProposalStore) StoreAgentProposal(_ context.Context, proposal types.StoredAgentProposal) error {
	if store.err != nil {
		return store.err
	}
	store.proposals = append(store.proposals, proposal)
	return nil
}

func (store *fakeProposalStore) ApproveAgentProposal(
	context.Context,
	types.ApproveAgentProposalCommand,
	string,
) (types.ApproveAgentProposalResult, error) {
	return types.ApproveAgentProposalResult{}, types.ErrAgentUnavailable
}

func (store *fakeProposalStore) VerifyApprovedAgentProposal(
	context.Context,
	types.VerifyApprovedAgentProposalCommand,
) (types.VerifyApprovedAgentProposalResult, error) {
	return types.VerifyApprovedAgentProposalResult{}, types.ErrAgentUnavailable
}

func (fake fakeProvider) GenerateProposal(
	context.Context,
	types.AgentProposalGenerationRequest,
) (types.AgentProposalGenerationResult, error) {
	return fake.result, fake.err
}
