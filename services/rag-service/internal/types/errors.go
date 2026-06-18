package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument      = errors.New("invalid rag request")
	ErrPermissionDenied     = errors.New("permission denied")
	ErrRetrievalUnavailable = errors.New("retrieval unavailable")
	ErrRAGUnavailable       = errors.New("rag unavailable")
	ErrCitationVerification = errors.New("citation verification failed")
)

func NewInvalidArgument(reason string) error {
	if reason == "" {
		return ErrInvalidArgument
	}
	return fmt.Errorf("%w: %s", ErrInvalidArgument, reason)
}
