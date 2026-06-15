package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument      = errors.New("invalid argument")
	ErrPermissionDenied     = errors.New("permission denied")
	ErrCursorRegression     = errors.New("cursor regression")
	ErrAckOutOfVisibleRange = errors.New("ack out of visible range")
	ErrInboxItemNotFound    = errors.New("inbox item not found")
	ErrDBReadFailed         = errors.New("db read failed")
	ErrDBWriteFailed        = errors.New("db write failed")
	ErrServiceOverloaded    = errors.New("service overloaded")
	ErrProjectionDependency = errors.New("projection dependency missing")
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

func NewCursorRegression(reason string) error {
	if reason == "" {
		return ErrCursorRegression
	}
	return fmt.Errorf("%w: %s", ErrCursorRegression, reason)
}

func NewAckOutOfVisibleRange(reason string) error {
	if reason == "" {
		return ErrAckOutOfVisibleRange
	}
	return fmt.Errorf("%w: %s", ErrAckOutOfVisibleRange, reason)
}

func NewInboxItemNotFound(reason string) error {
	if reason == "" {
		return ErrInboxItemNotFound
	}
	return fmt.Errorf("%w: %s", ErrInboxItemNotFound, reason)
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

func NewProjectionDependencyMissing(reason string) error {
	if reason == "" {
		return ErrProjectionDependency
	}
	return fmt.Errorf("%w: %s", ErrProjectionDependency, reason)
}
