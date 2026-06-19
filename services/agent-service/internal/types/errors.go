package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument          = errors.New("invalid argument")
	ErrPermissionDenied         = errors.New("permission denied")
	ErrRetrievalUnavailable     = errors.New("retrieval unavailable")
	ErrToolPolicyUnavailable    = errors.New("tool policy unavailable")
	ErrToolPrepareUnavailable   = errors.New("tool prepare unavailable")
	ErrAgentUnavailable         = errors.New("agent unavailable")
	ErrCitationVerification     = errors.New("citation verification failed")
	ErrProposalStoreUnavailable = errors.New("proposal store unavailable")
	ErrProposalNotFound         = errors.New("proposal not found")
	ErrProposalNotApprovable    = errors.New("proposal not approvable")
	ErrProposalNotApproved      = errors.New("proposal not approved")
	ErrProposalMismatch         = errors.New("proposal mismatch")
)

func NewInvalidArgument(reason string) error {
	if reason == "" {
		return ErrInvalidArgument
	}
	return fmt.Errorf("%w: %s", ErrInvalidArgument, reason)
}
