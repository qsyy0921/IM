package types

import (
	"errors"
	"fmt"
	"time"
)

type AdmissionPermit interface {
	Release()
}

var (
	ErrPermissionDenied       = errors.New("permission denied")
	ErrSequencerUnavailable   = errors.New("sequencer unavailable")
	ErrIdempotencyConflict    = errors.New("idempotency conflict")
	ErrUnsupportedMessageType = errors.New("unsupported message type")
	ErrUnsupportedDeleteScope = errors.New("unsupported delete scope")
	ErrConversationNotFound   = errors.New("conversation not found")
	ErrMessageNotFound        = errors.New("message not found")
	ErrInvalidMessageState    = errors.New("invalid message state")
	ErrDBWriteFailed          = errors.New("db write failed")
	ErrOutboxWriteFailed      = errors.New("outbox write failed")
	ErrServiceOverloaded      = errors.New("service overloaded")
	ErrDependencyVersion      = errors.New("dependency version mismatch")
	ErrDependencyUnavailable  = errors.New("dependency unavailable")
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

func NewUnsupportedDeleteScope(reason string) error {
	if reason == "" {
		return ErrUnsupportedDeleteScope
	}
	return fmt.Errorf("%w: %s", ErrUnsupportedDeleteScope, reason)
}

func NewConversationNotFound(reason string) error {
	if reason == "" {
		return ErrConversationNotFound
	}
	return fmt.Errorf("%w: %s", ErrConversationNotFound, reason)
}

func NewMessageNotFound(reason string) error {
	if reason == "" {
		return ErrMessageNotFound
	}
	return fmt.Errorf("%w: %s", ErrMessageNotFound, reason)
}

func NewInvalidMessageState(reason string) error {
	if reason == "" {
		return ErrInvalidMessageState
	}
	return fmt.Errorf("%w: %s", ErrInvalidMessageState, reason)
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
	return ServiceOverloadedError{Reason: reason}
}

func NewServiceOverloadedWithRetryDelay(reason string, retryDelay time.Duration) error {
	if retryDelay <= 0 {
		return NewServiceOverloaded(reason)
	}
	return ServiceOverloadedError{
		Reason:     reason,
		RetryDelay: retryDelay,
	}
}

type ServiceOverloadedError struct {
	Reason     string
	RetryDelay time.Duration
}

func (e ServiceOverloadedError) Error() string {
	if e.Reason == "" {
		return ErrServiceOverloaded.Error()
	}
	return fmt.Sprintf("%s: %s", ErrServiceOverloaded, e.Reason)
}

func (e ServiceOverloadedError) Unwrap() error {
	return ErrServiceOverloaded
}

func ServiceOverloadedRetryDelay(err error) (time.Duration, bool) {
	var overloaded ServiceOverloadedError
	if !errors.As(err, &overloaded) || overloaded.RetryDelay <= 0 {
		return 0, false
	}
	return overloaded.RetryDelay, true
}

func NewDependencyVersionMismatch(reason string) error {
	if reason == "" {
		return ErrDependencyVersion
	}
	return fmt.Errorf("%w: %s", ErrDependencyVersion, reason)
}

func NewDependencyUnavailable(reason string) error {
	if reason == "" {
		return ErrDependencyUnavailable
	}
	return fmt.Errorf("%w: %s", ErrDependencyUnavailable, reason)
}
