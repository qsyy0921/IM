package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

type ExecuteApprovedActionUseCase struct {
	catalog  SkillCatalogPort
	policy   ToolPolicyPort
	approval ProposalApprovalPort
	audit    ExecutionAuditRepository
}

func NewExecuteApprovedActionUseCase(
	catalog SkillCatalogPort,
	policy ToolPolicyPort,
	approval ProposalApprovalPort,
	audit ExecutionAuditRepository,
) ExecuteApprovedActionUseCase {
	return ExecuteApprovedActionUseCase{catalog: catalog, policy: policy, approval: approval, audit: audit}
}

func (usecase ExecuteApprovedActionUseCase) Execute(
	ctx context.Context,
	command types.ExecuteApprovedActionCommand,
) (types.ExecuteApprovedActionResult, error) {
	if err := command.Validate(); err != nil {
		return types.ExecuteApprovedActionResult{}, err
	}
	command = command.Normalized()
	if usecase.catalog == nil {
		return types.ExecuteApprovedActionResult{}, types.ErrSkillCatalogUnavailable
	}
	if usecase.policy == nil {
		return types.ExecuteApprovedActionResult{}, types.ErrToolPolicyUnavailable
	}
	if usecase.approval == nil {
		return types.ExecuteApprovedActionResult{}, types.ErrProposalApprovalUnavailable
	}
	if usecase.audit == nil {
		return types.ExecuteApprovedActionResult{}, types.ErrExecutionAuditFailed
	}

	if _, err := usecase.approval.VerifyApprovedProposal(ctx, types.VerifyApprovedProposalCommand{
		AuthContext:     command.AuthContext,
		ProposalID:      command.ProposalID,
		ApprovalID:      command.ApprovalID,
		PreparedAuditID: command.PreparedAuditID,
		SkillID:         command.SkillID,
		ToolName:        command.ToolName,
		ResourceType:    command.ResourceType,
		ResourceID:      command.ResourceID,
	}); err != nil {
		return types.ExecuteApprovedActionResult{}, err
	}

	skill, err := usecase.catalog.GetSkill(ctx, command.AuthContext, command.SkillID)
	if err != nil {
		return types.ExecuteApprovedActionResult{}, err
	}
	if strings.TrimSpace(skill.Status) != types.SkillStatusActive {
		result := blockedResult(command, skill, "SKILL_DISABLED", "skill disabled")
		return usecase.insertAudit(ctx, command, result, types.ExecutionStatusBlocked)
	}
	if strings.TrimSpace(skill.ToolName) != command.ToolName {
		result := blockedResult(command, skill, "TOOL_MISMATCH", "tool does not match skill")
		return usecase.insertAudit(ctx, command, result, types.ExecutionStatusBlocked)
	}
	if !types.ToolActionAllowed(skill.AllowedActions, types.ToolActionExecute) {
		result := blockedResult(command, skill, "ACTION_NOT_ALLOWED", "tool execute action not allowed")
		return usecase.insertAudit(ctx, command, result, types.ExecutionStatusBlocked)
	}

	decision, err := usecase.policy.CheckToolAction(ctx, types.CheckToolActionCommand{
		AuthContext:  command.AuthContext,
		ToolName:     command.ToolName,
		Action:       types.ToolActionExecute,
		ResourceType: command.ResourceType,
		ResourceID:   command.ResourceID,
		RiskLevel:    effectiveRiskLevel(command.RiskLevel, skill.RiskLevel),
		Intent:       command.Intent,
	})
	if err != nil {
		return types.ExecuteApprovedActionResult{}, err
	}
	result := resultFromDecision(command, skill, decision)
	status := types.ExecutionStatusRecorded
	if !result.Allowed {
		status = types.ExecutionStatusBlocked
	}
	return usecase.insertAudit(ctx, command, result, status)
}

func (usecase ExecuteApprovedActionUseCase) insertAudit(
	ctx context.Context,
	command types.ExecuteApprovedActionCommand,
	result types.ExecuteApprovedActionResult,
	status string,
) (types.ExecuteApprovedActionResult, error) {
	result.Status = status
	result.ExecutionID = newExecutionID()
	audit := types.ExecutionAudit{
		TenantID:          command.AuthContext.TenantID,
		ExecutionID:       result.ExecutionID,
		ProposalID:        command.ProposalID,
		ApprovalID:        command.ApprovalID,
		PreparedAuditID:   command.PreparedAuditID,
		UserID:            command.AuthContext.UserID,
		DeviceID:          command.AuthContext.DeviceID,
		SessionID:         command.AuthContext.SessionID,
		TraceID:           command.AuthContext.TraceID,
		RequestID:         command.AuthContext.RequestID,
		SkillID:           command.SkillID,
		ToolName:          command.ToolName,
		Action:            command.Action,
		ResourceType:      command.ResourceType,
		ResourceID:        command.ResourceID,
		RiskLevel:         result.RiskLevel,
		Intent:            command.Intent,
		IdempotencyKey:    command.IdempotencyKey,
		InputSHA256:       command.InputSHA256(),
		Allowed:           result.Allowed,
		RequiresApproval:  result.RequiresApproval,
		PermissionVersion: result.PermissionVersion,
		Classification:    result.Classification,
		Reason:            result.Reason,
		DecisionSource:    result.DecisionSource,
		Status:            status,
		Executed:          result.Executed,
	}
	if err := usecase.audit.InsertExecutionAudit(ctx, audit); err != nil {
		return types.ExecuteApprovedActionResult{}, err
	}
	return result, nil
}

func blockedResult(
	command types.ExecuteApprovedActionCommand,
	skill types.SkillDefinition,
	classification string,
	reason string,
) types.ExecuteApprovedActionResult {
	return types.ExecuteApprovedActionResult{
		TenantID:         command.AuthContext.TenantID,
		UserID:           command.AuthContext.UserID,
		ProposalID:       command.ProposalID,
		ApprovalID:       command.ApprovalID,
		PreparedAuditID:  command.PreparedAuditID,
		SkillID:          command.SkillID,
		ToolName:         command.ToolName,
		Action:           types.ToolActionExecute,
		ResourceType:     command.ResourceType,
		ResourceID:       command.ResourceID,
		RiskLevel:        effectiveRiskLevel(command.RiskLevel, skill.RiskLevel),
		Allowed:          false,
		RequiresApproval: true,
		Classification:   classification,
		Reason:           reason,
		DecisionSource:   "action-executor",
		Executed:         false,
		OutputJSON:       "{}",
	}
}

func resultFromDecision(
	command types.ExecuteApprovedActionCommand,
	skill types.SkillDefinition,
	decision types.ToolPolicyDecision,
) types.ExecuteApprovedActionResult {
	return types.ExecuteApprovedActionResult{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ProposalID:        command.ProposalID,
		ApprovalID:        command.ApprovalID,
		PreparedAuditID:   command.PreparedAuditID,
		SkillID:           command.SkillID,
		ToolName:          command.ToolName,
		Action:            types.ToolActionExecute,
		ResourceType:      command.ResourceType,
		ResourceID:        command.ResourceID,
		RiskLevel:         effectiveRiskLevel(command.RiskLevel, skill.RiskLevel),
		Allowed:           decision.Allowed,
		RequiresApproval:  skill.RequiresApproval || decision.RequiresApproval,
		PermissionVersion: decision.PermissionVersion,
		Classification:    decision.Classification,
		Reason:            decision.Reason,
		DecisionSource:    decision.DecisionSource,
		Executed:          false,
		OutputJSON:        "{}",
	}
}

func effectiveRiskLevel(requested string, skillRisk string) string {
	requested = strings.ToUpper(strings.TrimSpace(requested))
	if requested != "" {
		return requested
	}
	return strings.ToUpper(strings.TrimSpace(skillRisk))
}

func newExecutionID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "act_exec_fallback"
	}
	return "act_exec_" + hex.EncodeToString(buf[:])
}

func PublicBlockedError(result types.ExecuteApprovedActionResult) error {
	if result.Classification == "SKILL_DISABLED" {
		return types.ErrSkillDisabled
	}
	if result.Classification == "ACTION_NOT_ALLOWED" {
		return types.ErrToolActionNotAllowed
	}
	return fmt.Errorf("%w: %s", types.ErrToolPolicyDenied, result.Reason)
}
