package types

import (
	"errors"
	"fmt"
)

var (
	ErrPermissionDenied       = errors.New("permission denied")
	ErrSequencerUnavailable   = errors.New("sequencer unavailable")
	ErrIdempotencyConflict    = errors.New("idempotency conflict")
	ErrUnsupportedMessageType = errors.New("unsupported message type")
	ErrDBWriteFailed          = errors.New("db write failed")
	ErrOutboxWriteFailed      = errors.New("outbox write failed")
	ErrServiceOverloaded      = errors.New("service overloaded")
	ErrDependencyVersion      = errors.New("dependency version mismatch")
)

func NewPermissionDenied(reason string) error {
	if reason == "" {
		return ErrPermissionDenied
	}
	return fmt.Errorf("%w: %s", ErrPermissionDenied, reason)
}

func NewSequencerUnavailable(reason string) error {
	if reason == "" {
		return ErrSequencerUnavailable
	}
	return fmt.Errorf("%w: %s", ErrSequencerUnavailable, reason)
}

func NewIdempotencyConflict(reason string) error {
	if reason == "" {
		return ErrIdempotencyConflict
	}
	return fmt.Errorf("%w: %s", ErrIdempotencyConflict, reason)
}

func NewUnsupportedMessageType(reason string) error {
	if reason == "" {
		return ErrUnsupportedMessageType
	}
	return fmt.Errorf("%w: %s", ErrUnsupportedMessageType, reason)
}

func NewDBWriteFailed(reason string) error {
	if reason == "" {
		return ErrDBWriteFailed
	}
	return fmt.Errorf("%w: %s", ErrDBWriteFailed, reason)
}

func NewOutboxWriteFailed(reason string) error {
	if reason == "" {
		return ErrOutboxWriteFailed
	}
	return fmt.Errorf("%w: %s", ErrOutboxWriteFailed, reason)
}

func NewServiceOverloaded(reason string) error {
	if reason == "" {
		return ErrServiceOverloaded
	}
	return fmt.Errorf("%w: %s", ErrServiceOverloaded, reason)
}

func NewDependencyVersionMismatch(reason string) error {
	if reason == "" {
		return ErrDependencyVersion
	}
	return fmt.Errorf("%w: %s", ErrDependencyVersion, reason)
}
