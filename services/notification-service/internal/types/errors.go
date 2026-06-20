package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument    = errors.New("invalid argument")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrAlreadyExists      = errors.New("already exists")
	ErrNotFound           = errors.New("not found")
	ErrFailedPrecondition = errors.New("failed precondition")
	ErrDBReadFailed       = errors.New("db read failed")
	ErrDBWriteFailed      = errors.New("db write failed")
	ErrDependencyFailed   = errors.New("dependency failed")
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

func NewAlreadyExists(reason string) error {
	if reason == "" {
		return ErrAlreadyExists
	}
	return fmt.Errorf("%w: %s", ErrAlreadyExists, reason)
}

func NewNotFound(reason string) error {
	if reason == "" {
		return ErrNotFound
	}
	return fmt.Errorf("%w: %s", ErrNotFound, reason)
}

func NewFailedPrecondition(reason string) error {
	if reason == "" {
		return ErrFailedPrecondition
	}
	return fmt.Errorf("%w: %s", ErrFailedPrecondition, reason)
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

func NewDependencyFailed(reason string) error {
	if reason == "" {
		return ErrDependencyFailed
	}
	return fmt.Errorf("%w: %s", ErrDependencyFailed, reason)
}
