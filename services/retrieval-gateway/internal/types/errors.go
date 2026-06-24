package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument      = errors.New("invalid retrieval request")
	ErrPermissionDenied     = errors.New("permission denied")
	ErrRetrievalUnavailable = errors.New("retrieval unavailable")
	ErrSearchUnavailable    = errors.New("search unavailable")
	ErrMemoryUnavailable    = errors.New("memory unavailable")
	ErrVectorUnavailable    = errors.New("vector unavailable")
)

func NewInvalidArgument(reason string) error {
	if reason == "" {
		return ErrInvalidArgument
	}
	return fmt.Errorf("%w: %s", ErrInvalidArgument, reason)
}

func NewRetrievalUnavailable(reason string) error {
	if reason == "" {
		return ErrRetrievalUnavailable
	}
	return fmt.Errorf("%w: %s", ErrRetrievalUnavailable, reason)
}
