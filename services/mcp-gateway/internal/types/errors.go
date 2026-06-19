package types

import "errors"

var (
	ErrInvalidArgument         = errors.New("invalid argument")
	ErrPermissionDenied        = errors.New("permission denied")
	ErrSkillCatalogUnavailable = errors.New("skill catalog unavailable")
	ErrSkillNotFound           = errors.New("skill not found")
	ErrSkillDisabled           = errors.New("skill disabled")
	ErrToolActionNotAllowed    = errors.New("tool action not allowed")
	ErrToolPolicyUnavailable   = errors.New("tool policy unavailable")
	ErrToolPolicyDenied        = errors.New("tool policy denied")
	ErrAuditWriteFailed        = errors.New("mcp audit write failed")
)
