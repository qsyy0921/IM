package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

func TestRepositoryAgentProposalApprovalIntegration(t *testing.T) {
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, mustReadMigration(t)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM agent_proposals WHERE tenant_id = 'tenant-agent-pg-test'`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	repository := NewRepository(pool)
	proposal := testStoredProposal()
	if err := repository.StoreAgentProposal(ctx, proposal); err != nil {
		t.Fatalf("store proposal: %v", err)
	}
	approval, err := repository.ApproveAgentProposal(ctx, types.ApproveAgentProposalCommand{
		AuthContext: types.AuthContext{TenantID: proposal.TenantID, UserID: "approver-1"},
		ProposalID:  proposal.ProposalID,
		Reason:      "looks grounded",
	}, "appr-pg-1")
	if err != nil {
		t.Fatalf("approve proposal: %v", err)
	}
	if approval.Status != types.AgentProposalStatusApproved || approval.ApprovalID != "appr-pg-1" {
		t.Fatalf("unexpected approval: %+v", approval)
	}
	verified, err := repository.VerifyApprovedAgentProposal(ctx, types.VerifyApprovedAgentProposalCommand{
		AuthContext:     types.AuthContext{TenantID: proposal.TenantID, UserID: proposal.UserID},
		ProposalID:      proposal.ProposalID,
		ApprovalID:      "appr-pg-1",
		PreparedAuditID: proposal.PreparedAuditID,
		SkillID:         proposal.SkillID,
		ToolName:        proposal.ToolName,
		ResourceType:    proposal.ResourceType,
		ResourceID:      proposal.ResourceID,
	}.Normalized())
	if err != nil {
		t.Fatalf("verify approved proposal: %v", err)
	}
	if verified.Status != types.AgentProposalStatusApproved || verified.PreparedAuditID != proposal.PreparedAuditID {
		t.Fatalf("unexpected verify result: %+v", verified)
	}
	_, err = repository.VerifyApprovedAgentProposal(ctx, types.VerifyApprovedAgentProposalCommand{
		AuthContext:     types.AuthContext{TenantID: proposal.TenantID, UserID: proposal.UserID},
		ProposalID:      proposal.ProposalID,
		ApprovalID:      "appr-pg-1",
		PreparedAuditID: proposal.PreparedAuditID,
		SkillID:         proposal.SkillID,
		ToolName:        proposal.ToolName,
		ResourceType:    proposal.ResourceType,
		ResourceID:      "other-conv",
	}.Normalized())
	if !errors.Is(err, types.ErrProposalMismatch) {
		t.Fatalf("expected proposal mismatch, got %v", err)
	}
}

func testStoredProposal() types.StoredAgentProposal {
	return types.StoredAgentProposal{
		TenantID:          "tenant-agent-pg-test",
		ProposalID:        "ap_pg_1",
		UserID:            "user-1",
		ConversationID:    "conv-1",
		Objective:         "draft an approved action",
		SkillID:           "conversation.note.create",
		PreparedAuditID:   "mcp-audit-pg-1",
		ToolName:          "conversation.note.create",
		ToolAction:        types.ToolActionCall,
		ResourceType:      "conversation",
		ResourceID:        "conv-1",
		RiskLevel:         "LOW",
		Intent:            "draft note",
		Status:            types.AgentProposalStatusProposed,
		ProposalText:      "proposal",
		RequiresApproval:  true,
		Allowed:           true,
		PermissionVersion: 3,
		Classification:    "TOOL_ALLOWED",
		Reason:            "allowed",
		DecisionSource:    "test",
		EvidencePackID:    "pack-1",
		CitationsJSON:     `[{"evidence_id":"e1"}]`,
		AgentVersion:      types.AgentVersion,
		GeneratedByLLM:    false,
	}
}

func mustReadMigration(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", ".."))
	path := filepath.Join(root, "migrations", "postgres", "agent-service", "000001_agent_proposals.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	return string(content)
}
