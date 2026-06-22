package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/services/mcp-gateway/internal/types"
)

type PrepareToolCallUseCase struct {
	catalog SkillCatalogPort
	policy  ToolPolicyPort
	audit   AuditRepository
}

func NewPrepareToolCallUseCase(
	catalog SkillCatalogPort,
	policy ToolPolicyPort,
	audit AuditRepository,
) PrepareToolCallUseCase {
	return PrepareToolCallUseCase{catalog: catalog, policy: policy, audit: audit}
}

func (usecase PrepareToolCallUseCase) Execute(
	ctx context.Context,
	command types.PrepareToolCallCommand,
) (types.PrepareToolCallResult, error) {
	if err := command.Validate(); err != nil {
		return types.PrepareToolCallResult{}, err
	}
	command = command.Normalized()
	if usecase.catalog == nil {
		return types.PrepareToolCallResult{}, types.ErrSkillCatalogUnavailable
	}
	if usecase.policy == nil {
		return types.PrepareToolCallResult{}, types.ErrToolPolicyUnavailable
	}
	if usecase.audit == nil {
		return types.PrepareToolCallResult{}, types.ErrAuditWriteFailed
	}

	skill, err := usecase.catalog.GetSkill(ctx, command.AuthContext, command.SkillID)
	if err != nil {
		return types.PrepareToolCallResult{}, err
	}
	if strings.TrimSpace(skill.Status) != types.SkillStatusActive {
		result := blockedResult(command, skill, "SKILL_DISABLED", "skill disabled")
		auditID, err := usecase.insertAudit(ctx, command, result, types.ToolAuditStatusBlocked)
		result.AuditID = auditID
		return result, err
	}
	if strings.TrimSpace(skill.ToolName) != command.ToolName {
		result := blockedResult(command, skill, "TOOL_MISMATCH", "tool does not match skill")
		auditID, err := usecase.insertAudit(ctx, command, result, types.ToolAuditStatusBlocked)
		result.AuditID = auditID
		return result, err
	}
	if !types.ToolActionAllowed(skill.AllowedActions, command.Action) {
		result := blockedResult(command, skill, "ACTION_NOT_ALLOWED", "tool action not allowed")
		auditID, err := usecase.insertAudit(ctx, command, result, types.ToolAuditStatusBlocked)
		if err != nil {
			return types.PrepareToolCallResult{}, err
		}
		result.AuditID = auditID
		return result, nil
	}

	decision, err := usecase.policy.CheckToolAction(ctx, types.CheckToolActionCommand{
		AuthContext:  command.AuthContext,
		ToolName:     command.ToolName,
		Action:       command.Action,
		ResourceType: command.ResourceType,
		ResourceID:   command.ResourceID,
		RiskLevel:    effectiveRiskLevel(command.RiskLevel, skill.RiskLevel),
		Intent:       command.Intent,
	})
	if err != nil {
		return types.PrepareToolCallResult{}, err
	}
	result := resultFromDecision(command, skill, decision)
	status := types.ToolAuditStatusAllowed
	if !result.Allowed {
		status = types.ToolAuditStatusBlocked
	}
	auditID, err := usecase.insertAudit(ctx, command, result, status)
	if err != nil {
		return types.PrepareToolCallResult{}, err
	}
	result.AuditID = auditID
	return result, nil
}

func (usecase PrepareToolCallUseCase) insertAudit(
	ctx context.Context,
	command types.PrepareToolCallCommand,
	result types.PrepareToolCallResult,
	status string,
) (string, error) {
	audit := types.ToolCallAudit{
		TenantID:          command.AuthContext.TenantID,
		AuditID:           newAuditID(),
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
	}
	if err := usecase.audit.InsertToolCallAudit(ctx, audit); err != nil {
		return "", err
	}
	return audit.AuditID, nil
}

func blockedResult(
	command types.PrepareToolCallCommand,
	skill types.SkillDefinition,
	classification string,
	reason string,
) types.PrepareToolCallResult {
	return types.PrepareToolCallResult{
		TenantID:         command.AuthContext.TenantID,
		UserID:           command.AuthContext.UserID,
		SkillID:          command.SkillID,
		ToolName:         command.ToolName,
		Action:           command.Action,
		ResourceType:     command.ResourceType,
		ResourceID:       command.ResourceID,
		RiskLevel:        effectiveRiskLevel(command.RiskLevel, skill.RiskLevel),
		Allowed:          false,
		RequiresApproval: skill.RequiresApproval,
		Classification:   classification,
		Reason:           reason,
		DecisionSource:   "mcp-gateway",
	}
}

func resultFromDecision(
	command types.PrepareToolCallCommand,
	skill types.SkillDefinition,
	decision types.ToolPolicyDecision,
) types.PrepareToolCallResult {
	requiresApproval := skill.RequiresApproval || decision.RequiresApproval
	return types.PrepareToolCallResult{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		SkillID:           command.SkillID,
		ToolName:          command.ToolName,
		Action:            command.Action,
		ResourceType:      command.ResourceType,
		ResourceID:        command.ResourceID,
		RiskLevel:         effectiveRiskLevel(command.RiskLevel, skill.RiskLevel),
		Allowed:           decision.Allowed,
		RequiresApproval:  requiresApproval,
		PermissionVersion: decision.PermissionVersion,
		Classification:    decision.Classification,
		Reason:            decision.Reason,
		DecisionSource:    decision.DecisionSource,
	}
}

func effectiveRiskLevel(requested string, skillRisk string) string {
	requested = strings.ToUpper(strings.TrimSpace(requested))
	if requested != "" {
		return requested
	}
	return strings.ToUpper(strings.TrimSpace(skillRisk))
}

func newAuditID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "mcp_audit_recovery"
	}
	return "mcp_audit_" + hex.EncodeToString(buf[:])
}

func publicBlockedError(result types.PrepareToolCallResult) error {
	if result.Classification == "SKILL_DISABLED" {
		return types.ErrSkillDisabled
	}
	if result.Classification == "ACTION_NOT_ALLOWED" {
		return types.ErrToolActionNotAllowed
	}
	return fmt.Errorf("%w: %s", types.ErrToolPolicyDenied, result.Reason)
}
