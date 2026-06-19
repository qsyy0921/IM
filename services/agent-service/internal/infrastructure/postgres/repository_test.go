package postgres

import (
	"context"
	"encoding/json"
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
	if _, err := pool.Exec(ctx, `DELETE FROM agent_approval_outbox WHERE tenant_id = 'tenant-agent-pg-test'`); err != nil {
		t.Fatalf("cleanup approval outbox: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM agent_proposals WHERE tenant_id = 'tenant-agent-pg-test'`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	repository := NewRepository(pool)
	proposal := testStoredProposal()
	if err := repository.StoreAgentProposal(ctx, proposal); err != nil {
		t.Fatalf("store proposal: %v", err)
	}
	auditRows, err := repository.AuditAgentProposalApprovals(ctx, AgentProposalApprovalAuditOptions{
		TenantID:   string(proposal.TenantID),
		ProposalID: proposal.ProposalID,
		Status:     types.AgentProposalStatusProposed,
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("audit proposal approvals: %v", err)
	}
	if len(auditRows) != 1 {
		t.Fatalf("expected one approval audit row, got %d", len(auditRows))
	}
	if auditRows[0].ProposalID != proposal.ProposalID ||
		auditRows[0].SkillID != proposal.SkillID ||
		auditRows[0].PreparedAuditID != proposal.PreparedAuditID ||
		auditRows[0].Status != types.AgentProposalStatusProposed ||
		!auditRows[0].RequiresApproval {
		t.Fatalf("unexpected approval audit row: %+v", auditRows[0])
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
	candidate, err := repository.GetAgentProposalApprovalCandidate(ctx, string(proposal.TenantID), proposal.ProposalID)
	if err != nil {
		t.Fatalf("get approved candidate: %v", err)
	}
	if candidate.Status != types.AgentProposalStatusApproved ||
		candidate.ApprovalID != "appr-pg-1" ||
		candidate.ApprovedByUserID != "approver-1" ||
		!candidate.ApprovalReasonPresent {
		t.Fatalf("unexpected approved candidate: %+v", candidate)
	}
	var outboxStatus string
	var eventType string
	var payload string
	err = pool.QueryRow(ctx, `
SELECT status, event_type, payload_json::text
FROM agent_approval_outbox
WHERE tenant_id = $1 AND approval_id = $2`,
		string(proposal.TenantID),
		"appr-pg-1",
	).Scan(&outboxStatus, &eventType, &payload)
	if err != nil {
		t.Fatalf("query approval outbox: %v", err)
	}
	if outboxStatus != "PENDING" || eventType != "agent.proposal.approved.v1" {
		t.Fatalf("unexpected approval outbox row status=%q event_type=%q payload=%s", outboxStatus, eventType, payload)
	}
	for _, required := range []string{
		`"approval_id": "appr-pg-1"`,
		`"prepared_audit_id": "mcp-audit-pg-1"`,
		`"tool_name": "conversation.note.create"`,
	} {
		if !strings.Contains(payload, required) {
			t.Fatalf("approval outbox payload missing %s: %s", required, payload)
		}
	}
	var payloadFields map[string]any
	if err := json.Unmarshal([]byte(payload), &payloadFields); err != nil {
		t.Fatalf("decode approval outbox payload: %v", err)
	}
	for _, forbidden := range []string{"objective", "proposal_text", "reason", "citations_json"} {
		if _, ok := payloadFields[forbidden]; ok {
			t.Fatalf("approval outbox payload includes forbidden field %q: %s", forbidden, payload)
		}
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
	migrationDir := filepath.Join(root, "migrations", "postgres", "agent-service")
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		t.Fatalf("read migration dir: %v", err)
	}
	var builder strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(migrationDir, entry.Name()))
		if err != nil {
			t.Fatalf("read migration %s: %v", entry.Name(), err)
		}
		builder.Write(content)
		builder.WriteString("\n")
	}
	return builder.String()
}
