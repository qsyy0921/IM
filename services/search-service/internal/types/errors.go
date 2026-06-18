package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument    = errors.New("invalid argument")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrSearchUnavailable  = errors.New("search unavailable")
	ErrProjectionStale    = errors.New("projection stale")
	ErrDBReadFailed       = errors.New("db read failed")
	ErrDBWriteFailed      = errors.New("db write failed")
	ErrServiceOverloaded  = errors.New("service overloaded")
	ErrUnsupportedPayload = errors.New("unsupported payload")
)

func NewInvalidArgument(reason string) error {
	if reason == "" {
		return ErrInvalidArgument
	}
	return fmt.Errorf("%w: %s", ErrInvalidArgument, reason)
}

func NewPermissionDenied(reason string) error {
	if reason == "" {
		return ErrPermissionDenied
	}
	return fmt.Errorf("%w: %s", ErrPermissionDenied, reason)
}

func NewSearchUnavailable(reason string) error {
	if reason == "" {
		return ErrSearchUnavailable
	}
	return fmt.Errorf("%w: %s", ErrSearchUnavailable, reason)
}

func NewProjectionStale(reason string) error {
	if reason == "" {
		return ErrProjectionStale
	}
	return fmt.Errorf("%w: %s", ErrProjectionStale, reason)
}

func NewDBReadFailed(reason string) error {
	if reason == "" {
		return ErrDBReadFailed
	}
	return fmt.Errorf("%w: %s", ErrDBReadFailed, reason)
}

func NewDBWriteFailed(reason string) error {
	if reason == "" {
		return ErrDBWriteFailed
	}
	return fmt.Errorf("%w: %s", ErrDBWriteFailed, reason)
}

func NewServiceOverloaded(reason string) error {
	if reason == "" {
		return ErrServiceOverloaded
	}
	return fmt.Errorf("%w: %s", ErrServiceOverloaded, reason)
}

func NewUnsupportedPayload(reason string) error {
	if reason == "" {
		return ErrUnsupportedPayload
	}
	return fmt.Errorf("%w: %s", ErrUnsupportedPayload, reason)
}
