package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument      = errors.New("invalid argument")
	ErrConversationNotFound = errors.New("conversation not found")
	ErrMemberChangeNotFound = errors.New("member change not found")
	ErrMemberNotActive      = errors.New("conversation member is not active")
	ErrMemberConflict       = errors.New("member conflict")
	ErrPermissionDenied     = errors.New("permission denied")
	ErrDBReadFailed         = errors.New("db read failed")
	ErrDBWriteFailed        = errors.New("db write failed")
	ErrOutboxWriteFailed    = errors.New("outbox write failed")
	ErrSequencerUnavailable = errors.New("sequencer unavailable")
)

func NewInvalidArgument(reason string) error {
	if reason == "" {
		return ErrInvalidArgument
	}
	return fmt.Errorf("%w: %s", ErrInvalidArgument, reason)
}

func NewConversationNotFound(reason string) error {
	if reason == "" {
		return ErrConversationNotFound
	}
	return fmt.Errorf("%w: %s", ErrConversationNotFound, reason)
}

func NewMemberChangeNotFound(reason string) error {
	if reason == "" {
		return ErrMemberChangeNotFound
	}
	return fmt.Errorf("%w: %s", ErrMemberChangeNotFound, reason)
}

func NewMemberNotActive(reason string) error {
	if reason == "" {
		return ErrMemberNotActive
	}
	return fmt.Errorf("%w: %s", ErrMemberNotActive, reason)
}

func NewMemberConflict(reason string) error {
	if reason == "" {
		return ErrMemberConflict
	}
	return fmt.Errorf("%w: %s", ErrMemberConflict, reason)
}

func NewPermissionDenied(reason string) error {
	if reason == "" {
		return ErrPermissionDenied
	}
	return fmt.Errorf("%w: %s", ErrPermissionDenied, reason)
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

func NewOutboxWriteFailed(reason string) error {
	if reason == "" {
		return ErrOutboxWriteFailed
	}
	return fmt.Errorf("%w: %s", ErrOutboxWriteFailed, reason)
}

func NewSequencerUnavailable(reason string) error {
	if reason == "" {
		return ErrSequencerUnavailable
	}
	return fmt.Errorf("%w: %s", ErrSequencerUnavailable, reason)
}
