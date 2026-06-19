package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/agent-service/internal/app"
	postgresinfra "github.com/qsyy0921/IM/services/agent-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

func runProposalApprovalAudit(ctx context.Context) error {
	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	options := postgresinfra.AgentProposalApprovalAuditOptions{
		TenantID:     envString("NEXUSIM_AGENT_PROPOSAL_APPROVAL_AUDIT_TENANT_ID", ""),
		ProposalID:   envString("NEXUSIM_AGENT_PROPOSAL_APPROVAL_AUDIT_PROPOSAL_ID", ""),
		UserID:       envString("NEXUSIM_AGENT_PROPOSAL_APPROVAL_AUDIT_USER_ID", ""),
		Status:       envString("NEXUSIM_AGENT_PROPOSAL_APPROVAL_AUDIT_STATUS", types.AgentProposalStatusProposed),
		ToolName:     envString("NEXUSIM_AGENT_PROPOSAL_APPROVAL_AUDIT_TOOL_NAME", ""),
		ResourceType: envString("NEXUSIM_AGENT_PROPOSAL_APPROVAL_AUDIT_RESOURCE_TYPE", ""),
		Limit:        envInt("NEXUSIM_AGENT_PROPOSAL_APPROVAL_AUDIT_LIMIT", 50),
	}
	rows, err := postgresinfra.NewRepository(pool).AuditAgentProposalApprovals(ctx, options)
	if err != nil {
		return err
	}
	log.Printf("agent-service proposal approval audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"agent_proposal tenant_id=%s proposal_id=%s user_id=%s skill_id=%s tool_name=%s resource_type=%s resource_id=%s status=%s requires_approval=%t approval_id=%s created_at=%s updated_at=%s",
			row.TenantID,
			row.ProposalID,
			row.UserID,
			row.SkillID,
			row.ToolName,
			row.ResourceType,
			row.ResourceID,
			row.Status,
			row.RequiresApproval,
			row.ApprovalID,
			row.CreatedAt.Format(time.RFC3339),
			row.UpdatedAt.Format(time.RFC3339),
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_AGENT_PROPOSAL_APPROVAL_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeProposalApprovalAuditOutput(outputPath, rows, map[string]string{
			"tenant_id":     options.TenantID,
			"proposal_id":   options.ProposalID,
			"user_id":       options.UserID,
			"status":        options.Status,
			"tool_name":     options.ToolName,
			"resource_type": options.ResourceType,
		}); err != nil {
			return err
		}
	}
	return nil
}

func runProposalApprovalApprove(ctx context.Context) error {
	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	tenantID := envString("NEXUSIM_AGENT_PROPOSAL_APPROVAL_TENANT_ID", "")
	proposalID := envString("NEXUSIM_AGENT_PROPOSAL_APPROVAL_PROPOSAL_ID", "")
	approvedByUserID := envString("NEXUSIM_AGENT_PROPOSAL_APPROVAL_APPROVED_BY_USER_ID", "")
	if tenantID == "" || proposalID == "" || approvedByUserID == "" {
		return fmt.Errorf("%w: tenant_id, proposal_id and approved_by_user_id are required", types.ErrInvalidArgument)
	}
	reason, err := agentOperatorReasonFromEnv(
		"NEXUSIM_AGENT_PROPOSAL_APPROVAL_REASON",
		"NEXUSIM_AGENT_PROPOSAL_APPROVAL_REASON_FILE",
		"agent proposal approval",
	)
	if err != nil {
		return err
	}
	dryRun, err := proposalApprovalDryRunFromEnv()
	if err != nil {
		return err
	}

	repository := postgresinfra.NewRepository(pool)
	request := proposalApprovalRequest{
		TenantID:              tenantID,
		ProposalID:            proposalID,
		ApprovedByUserID:      approvedByUserID,
		ApprovalReasonPresent: reason != "",
	}
	candidate, err := repository.GetAgentProposalApprovalCandidate(ctx, tenantID, proposalID)
	if err != nil {
		return err
	}
	if dryRun {
		log.Printf("agent-service proposal approval dry-run tenant_id=%s proposal_id=%s status=%s requires_approval=%t", tenantID, proposalID, candidate.Status, candidate.RequiresApproval)
		if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_AGENT_PROPOSAL_APPROVAL_OUTPUT")); outputPath != "" {
			if err := writeProposalApprovalApproveOutput(outputPath, true, request, &candidate, nil); err != nil {
				return err
			}
		}
		return nil
	}

	result, err := app.NewApproveAgentProposalUseCase(repository).Execute(ctx, types.ApproveAgentProposalCommand{
		AuthContext: types.AuthContext{
			TenantID: types.TenantID(tenantID),
			UserID:   types.UserID(approvedByUserID),
		},
		ProposalID: proposalID,
		Reason:     reason,
	})
	if err != nil {
		return err
	}
	log.Printf("agent-service proposal approved tenant_id=%s proposal_id=%s approval_id=%s approved_by_user_id=%s", tenantID, result.ProposalID, result.ApprovalID, result.ApprovedByUserID)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_AGENT_PROPOSAL_APPROVAL_OUTPUT")); outputPath != "" {
		if err := writeProposalApprovalApproveOutput(outputPath, false, request, &candidate, &result); err != nil {
			return err
		}
	}
	return nil
}

func proposalApprovalDryRunFromEnv() (bool, error) {
	value, set, err := envOptionalBool("NEXUSIM_AGENT_PROPOSAL_APPROVAL_DRY_RUN")
	if err != nil {
		return false, err
	}
	if !set {
		return true, nil
	}
	return value, nil
}
