package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument      = errors.New("invalid argument")
	ErrPermissionDenied     = errors.New("permission denied")
	ErrRetrievalUnavailable = errors.New("retrieval unavailable")
	ErrSummaryUnavailable   = errors.New("summary unavailable")
	ErrCitationVerification = errors.New("citation verification failed")
)

func NewInvalidArgument(reason string) error {
	if reason == "" {
		return ErrInvalidArgument
	}
	return fmt.Errorf("%w: %s", ErrInvalidArgument, reason)
}
