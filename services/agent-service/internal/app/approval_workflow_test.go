package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

func TestApproveAgentProposalUseCaseDelegatesToStore(t *testing.T) {
	store := &approvalStore{
		approveResult: types.ApproveAgentProposalResult{
			ProposalID:       "ap_1",
			ApprovalID:       "appr_1",
			Status:           types.AgentProposalStatusApproved,
			ApprovedByUserID: "approver-1",
			ApprovedAt:       time.Unix(171, 0),
		},
	}
	usecase := NewApproveAgentProposalUseCase(store)
	result, err := usecase.Execute(context.Background(), types.ApproveAgentProposalCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "approver-1", DeviceID: "device-1"},
		ProposalID:  "ap_1",
		Reason:      "approved",
	})
	if err != nil {
		t.Fatalf("approve proposal: %v", err)
	}
	if result.Status != types.AgentProposalStatusApproved || store.approvalID == "" {
		t.Fatalf("unexpected approval result=%+v approvalID=%q", result, store.approvalID)
	}
}

func TestApproveAgentProposalUseCaseRequiresStore(t *testing.T) {
	usecase := NewApproveAgentProposalUseCase(nil)
	_, err := usecase.Execute(context.Background(), types.ApproveAgentProposalCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "approver-1", DeviceID: "device-1"},
		ProposalID:  "ap_1",
	})
	if !errors.Is(err, types.ErrProposalStoreUnavailable) {
		t.Fatalf("expected store unavailable: %v", err)
	}
}

func TestVerifyApprovedAgentProposalUseCaseDelegatesToStore(t *testing.T) {
	store := &approvalStore{
		verifyResult: types.VerifyApprovedAgentProposalResult{
			ProposalID: "ap_1",
			ApprovalID: "appr_1",
			Status:     types.AgentProposalStatusApproved,
		},
	}
	usecase := NewVerifyApprovedAgentProposalUseCase(store)
	result, err := usecase.Execute(context.Background(), types.VerifyApprovedAgentProposalCommand{
		AuthContext:     types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		ProposalID:      "ap_1",
		ApprovalID:      "appr_1",
		PreparedAuditID: "mcp-audit-1",
		SkillID:         "skill-1",
		ToolName:        "conversation.note.create",
		ResourceType:    "conversation",
		ResourceID:      "conv-1",
	})
	if err != nil {
		t.Fatalf("verify approved proposal: %v", err)
	}
	if result.Status != types.AgentProposalStatusApproved || store.verifyCommand.ProposalID != "ap_1" {
		t.Fatalf("unexpected verify result=%+v command=%+v", result, store.verifyCommand)
	}
}

type approvalStore struct {
	approveResult types.ApproveAgentProposalResult
	verifyResult  types.VerifyApprovedAgentProposalResult
	approvalID    string
	verifyCommand types.VerifyApprovedAgentProposalCommand
}

func (store *approvalStore) StoreAgentProposal(context.Context, types.StoredAgentProposal) error {
	return nil
}

func (store *approvalStore) ApproveAgentProposal(
	_ context.Context,
	_ types.ApproveAgentProposalCommand,
	approvalID string,
) (types.ApproveAgentProposalResult, error) {
	store.approvalID = approvalID
	if store.approveResult.ApprovalID == "" {
		store.approveResult.ApprovalID = approvalID
	}
	return store.approveResult, nil
}

func (store *approvalStore) VerifyApprovedAgentProposal(
	_ context.Context,
	command types.VerifyApprovedAgentProposalCommand,
) (types.VerifyApprovedAgentProposalResult, error) {
	store.verifyCommand = command
	return store.verifyResult, nil
}
