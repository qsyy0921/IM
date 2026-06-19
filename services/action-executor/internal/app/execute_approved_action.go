package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

type ExecuteApprovedActionUseCase struct {
	catalog  SkillCatalogPort
	policy   ToolPolicyPort
	approval ProposalApprovalPort
	audit    ExecutionAuditRepository
	executor ToolExecutorPort
}

func NewExecuteApprovedActionUseCase(
	catalog SkillCatalogPort,
	policy ToolPolicyPort,
	approval ProposalApprovalPort,
	audit ExecutionAuditRepository,
) ExecuteApprovedActionUseCase {
	return ExecuteApprovedActionUseCase{catalog: catalog, policy: policy, approval: approval, audit: audit}
}

func NewExecuteApprovedActionUseCaseWithToolExecutor(
	catalog SkillCatalogPort,
	policy ToolPolicyPort,
	approval ProposalApprovalPort,
	audit ExecutionAuditRepository,
	executor ToolExecutorPort,
) ExecuteApprovedActionUseCase {
	return ExecuteApprovedActionUseCase{
		catalog:  catalog,
		policy:   policy,
		approval: approval,
		audit:    audit,
		executor: executor,
	}
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
	} else if usecase.executor != nil {
		execution, err := usecase.executor.ExecuteTool(ctx, types.ToolExecutionCommand{
			AuthContext:  command.AuthContext,
			Skill:        skill,
			ToolName:     command.ToolName,
			Action:       types.ToolActionExecute,
			ResourceType: command.ResourceType,
			ResourceID:   command.ResourceID,
			RiskLevel:    result.RiskLevel,
			Intent:       command.Intent,
			InputSHA256:  command.InputSHA256(),
		})
		switch {
		case err == nil:
			if execution.Executed {
				outputJSON, err := safeToolOutputJSON(execution.OutputJSON)
				if err != nil {
					status = types.ExecutionStatusFailed
					result.Classification = "TOOL_OUTPUT_UNSAFE"
					result.Reason = "tool output unsafe"
					result.DecisionSource = "action-executor"
					result.Executed = false
					result.OutputJSON = "{}"
					break
				}
				result.Executed = true
				result.OutputJSON = outputJSON
			}
		case errors.Is(err, types.ErrToolExecutionUnsupported):
			// Unsupported tools remain proposal/audit-only until a safe adapter is registered.
		default:
			status = types.ExecutionStatusFailed
			result.Classification, result.Reason = toolExecutionFailurePublicFields(err)
			result.DecisionSource = "action-executor"
			result.Executed = false
			result.OutputJSON = "{}"
		}
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
	result.ResultID = newResultID()
	result.ResultStatus = resultStatusForExecution(status, result.Executed)
	result.ResultRef = resultRef(result.ExecutionID, result.ResultID)
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
		OutputSHA256:      outputSHA256(result.OutputJSON, result.Executed),
	}
	projection := types.ToolResultProjection{
		TenantID:        command.AuthContext.TenantID,
		ResultID:        result.ResultID,
		ExecutionID:     result.ExecutionID,
		ProposalID:      command.ProposalID,
		ApprovalID:      command.ApprovalID,
		PreparedAuditID: command.PreparedAuditID,
		UserID:          command.AuthContext.UserID,
		SkillID:         command.SkillID,
		ToolName:        command.ToolName,
		ResourceType:    command.ResourceType,
		ResourceID:      command.ResourceID,
		Status:          result.ResultStatus,
		Executed:        result.Executed,
		ResultRef:       result.ResultRef,
		OutputSHA256:    audit.OutputSHA256,
	}
	if err := usecase.audit.RecordExecution(ctx, audit, projection); err != nil {
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

func newResultID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "act_result_fallback"
	}
	return "act_result_" + hex.EncodeToString(buf[:])
}

func resultStatusForExecution(status string, executed bool) string {
	if status == types.ExecutionStatusBlocked {
		return types.ResultStatusBlocked
	}
	if status == types.ExecutionStatusFailed {
		return types.ResultStatusFailed
	}
	if executed {
		return types.ResultStatusSucceeded
	}
	return types.ResultStatusNotExecuted
}

func resultRef(executionID string, resultID string) string {
	return "action-executor://executions/" + executionID + "/results/" + resultID
}

func outputSHA256(outputJSON string, executed bool) string {
	if !executed || strings.TrimSpace(outputJSON) == "" || strings.TrimSpace(outputJSON) == "{}" {
		return ""
	}
	sum := sha256.Sum256([]byte(outputJSON))
	return hex.EncodeToString(sum[:])
}

func safeToolOutputJSON(outputJSON string) (string, error) {
	outputJSON = strings.TrimSpace(outputJSON)
	if outputJSON == "" || len(outputJSON) > maxToolOutputJSONBytes {
		return "", types.ErrToolOutputUnsafe
	}
	var decoded any
	if err := json.Unmarshal([]byte(outputJSON), &decoded); err != nil {
		return "", types.ErrToolOutputUnsafe
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return "", types.ErrToolOutputUnsafe
	}
	if containsUnsafeToolOutputValue(object) {
		return "", types.ErrToolOutputUnsafe
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return "", types.ErrToolOutputUnsafe
	}
	return string(encoded), nil
}

const maxToolOutputJSONBytes = 16 * 1024

func containsUnsafeToolOutputValue(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if unsafeToolOutputKey(key) || containsUnsafeToolOutputValue(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if containsUnsafeToolOutputValue(nested) {
				return true
			}
		}
	case string:
		return unsafeToolOutputString(typed)
	}
	return false
}

func unsafeToolOutputKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	sensitiveKeys := []string{
		"authorization",
		"api_key",
		"access_key",
		"credential",
		"cookie",
		"email",
		"id_token",
		"password",
		"phone",
		"private_key",
		"refresh_token",
		"secret",
		"session",
		"ssn",
		"token",
	}
	for _, sensitive := range sensitiveKeys {
		if normalized == sensitive || strings.Contains(normalized, "_"+sensitive) || strings.Contains(normalized, sensitive+"_") {
			return true
		}
	}
	return false
}

func unsafeToolOutputString(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(value, "-----BEGIN ") && strings.Contains(value, " PRIVATE KEY-----") {
		return true
	}
	sensitiveFragments := []string{
		"authorization:",
		"bearer ",
		"password=",
		"refresh_token",
		"secret=",
		"sk-",
		"token=",
	}
	for _, fragment := range sensitiveFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return looksLikeEmail(value)
}

func looksLikeEmail(value string) bool {
	value = strings.TrimSpace(value)
	at := strings.Index(value, "@")
	if at <= 0 || at >= len(value)-3 {
		return false
	}
	domain := value[at+1:]
	return strings.Contains(domain, ".") && !strings.ContainsAny(value, " \t\r\n")
}

func toolExecutionFailurePublicFields(err error) (string, string) {
	switch {
	case errors.Is(err, types.ErrToolExecutionTimeout):
		return "TOOL_EXECUTION_TIMEOUT", "tool execution timeout"
	case errors.Is(err, types.ErrToolProviderUnavailable):
		return "TOOL_PROVIDER_UNAVAILABLE", "tool provider unavailable"
	case errors.Is(err, types.ErrToolProviderRateLimited):
		return "TOOL_PROVIDER_RATE_LIMITED", "tool provider rate limited"
	case errors.Is(err, types.ErrToolProviderPermissionDenied):
		return "TOOL_PROVIDER_PERMISSION_DENIED", "tool provider permission denied"
	case errors.Is(err, types.ErrToolOutputUnsafe):
		return "TOOL_OUTPUT_UNSAFE", "tool output unsafe"
	default:
		return "TOOL_EXECUTION_FAILED", "tool execution failed"
	}
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
