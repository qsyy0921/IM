package types

import "errors"

var (
	ErrInvalidArgument              = errors.New("invalid argument")
	ErrPermissionDenied             = errors.New("permission denied")
	ErrSkillCatalogUnavailable      = errors.New("skill catalog unavailable")
	ErrSkillNotFound                = errors.New("skill not found")
	ErrSkillDisabled                = errors.New("skill disabled")
	ErrToolActionNotAllowed         = errors.New("tool action not allowed")
	ErrToolPolicyUnavailable        = errors.New("tool policy unavailable")
	ErrToolPolicyDenied             = errors.New("tool policy denied")
	ErrExecutionAuditFailed         = errors.New("action execution audit failed")
	ErrProposalApprovalUnavailable  = errors.New("proposal approval unavailable")
	ErrProposalNotApproved          = errors.New("proposal not approved")
	ErrProposalMismatch             = errors.New("proposal mismatch")
	ErrToolExecutionUnsupported     = errors.New("tool execution unsupported")
	ErrToolExecutionFailed          = errors.New("tool execution failed")
	ErrToolExecutionTimeout         = errors.New("tool execution timeout")
	ErrToolProviderUnavailable      = errors.New("tool provider unavailable")
	ErrToolProviderRateLimited      = errors.New("tool provider rate limited")
	ErrToolProviderPermissionDenied = errors.New("tool provider permission denied")
	ErrToolOutputUnsafe             = errors.New("tool output unsafe")
)
