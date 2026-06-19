package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/agent-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

type proposalApprovalAuditOutput struct {
	GeneratedAt string                      `json:"generated_at"`
	Filters     map[string]string           `json:"filters"`
	Rows        []proposalApprovalOutputRow `json:"rows"`
}

type proposalApprovalApproveOutput struct {
	GeneratedAt string                     `json:"generated_at"`
	DryRun      bool                       `json:"dry_run"`
	Request     proposalApprovalRequest    `json:"request"`
	Candidate   *proposalApprovalOutputRow `json:"candidate,omitempty"`
	Result      *proposalApprovalResultRow `json:"result,omitempty"`
}

type proposalApprovalRequest struct {
	TenantID              string `json:"tenant_id"`
	ProposalID            string `json:"proposal_id"`
	ApprovedByUserID      string `json:"approved_by_user_id"`
	ApprovalReasonPresent bool   `json:"approval_reason_present"`
}

type proposalApprovalResultRow struct {
	ProposalID       string `json:"proposal_id"`
	ApprovalID       string `json:"approval_id"`
	Status           string `json:"status"`
	ApprovedByUserID string `json:"approved_by_user_id"`
	ApprovedAt       string `json:"approved_at"`
}

type proposalApprovalOutputRow struct {
	TenantID              string `json:"tenant_id"`
	ProposalID            string `json:"proposal_id"`
	UserID                string `json:"user_id"`
	ConversationID        string `json:"conversation_id"`
	SkillID               string `json:"skill_id"`
	PreparedAuditID       string `json:"prepared_audit_id"`
	ToolName              string `json:"tool_name"`
	ResourceType          string `json:"resource_type"`
	ResourceID            string `json:"resource_id"`
	RiskLevel             string `json:"risk_level"`
	Status                string `json:"status"`
	RequiresApproval      bool   `json:"requires_approval"`
	Allowed               bool   `json:"allowed"`
	PermissionVersion     int64  `json:"permission_version"`
	Classification        string `json:"classification"`
	DecisionSource        string `json:"decision_source"`
	EvidencePackID        string `json:"evidence_pack_id"`
	GeneratedByLLM        bool   `json:"generated_by_llm"`
	ApprovalID            string `json:"approval_id,omitempty"`
	ApprovedByUserID      string `json:"approved_by_user_id,omitempty"`
	ApprovalReasonPresent bool   `json:"approval_reason_present"`
	ApprovedAt            string `json:"approved_at,omitempty"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
}

func writeProposalApprovalAuditOutput(
	path string,
	rows []postgresinfra.AgentProposalApprovalAuditRow,
	filters map[string]string,
) error {
	output := proposalApprovalAuditOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Filters:     filters,
		Rows:        make([]proposalApprovalOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		output.Rows = append(output.Rows, proposalApprovalOutputRowFromRow(row))
	}
	return writeProposalApprovalJSON(path, output)
}

func writeProposalApprovalApproveOutput(
	path string,
	dryRun bool,
	request proposalApprovalRequest,
	candidate *postgresinfra.AgentProposalApprovalAuditRow,
	result *types.ApproveAgentProposalResult,
) error {
	output := proposalApprovalApproveOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		DryRun:      dryRun,
		Request:     request,
	}
	if candidate != nil {
		row := proposalApprovalOutputRowFromRow(*candidate)
		output.Candidate = &row
	}
	if result != nil {
		output.Result = &proposalApprovalResultRow{
			ProposalID:       result.ProposalID,
			ApprovalID:       result.ApprovalID,
			Status:           result.Status,
			ApprovedByUserID: string(result.ApprovedByUserID),
			ApprovedAt:       formatAgentOperatorTime(result.ApprovedAt),
		}
	}
	return writeProposalApprovalJSON(path, output)
}

func proposalApprovalOutputRowFromRow(row postgresinfra.AgentProposalApprovalAuditRow) proposalApprovalOutputRow {
	return proposalApprovalOutputRow{
		TenantID:              row.TenantID,
		ProposalID:            row.ProposalID,
		UserID:                row.UserID,
		ConversationID:        row.ConversationID,
		SkillID:               row.SkillID,
		PreparedAuditID:       row.PreparedAuditID,
		ToolName:              row.ToolName,
		ResourceType:          row.ResourceType,
		ResourceID:            row.ResourceID,
		RiskLevel:             row.RiskLevel,
		Status:                row.Status,
		RequiresApproval:      row.RequiresApproval,
		Allowed:               row.Allowed,
		PermissionVersion:     row.PermissionVersion,
		Classification:        row.Classification,
		DecisionSource:        row.DecisionSource,
		EvidencePackID:        row.EvidencePackID,
		GeneratedByLLM:        row.GeneratedByLLM,
		ApprovalID:            row.ApprovalID,
		ApprovedByUserID:      row.ApprovedByUserID,
		ApprovalReasonPresent: row.ApprovalReasonPresent,
		ApprovedAt:            formatAgentOperatorTime(row.ApprovedAt),
		CreatedAt:             formatAgentOperatorTime(row.CreatedAt),
		UpdatedAt:             formatAgentOperatorTime(row.UpdatedAt),
	}
}

func formatAgentOperatorTime(value time.Time) string {
	if value.IsZero() || value.Equal(time.Unix(0, 0).UTC()) {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func writeProposalApprovalJSON(path string, output any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
