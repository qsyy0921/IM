package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument  = errors.New("invalid argument")
	ErrPermissionDenied = errors.New("permission denied")
	ErrEvalRunNotFound  = errors.New("eval run not found")
	ErrDBReadFailed     = errors.New("db read failed")
	ErrDBWriteFailed    = errors.New("db write failed")
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

func NewEvalRunNotFound(reason string) error {
	if reason == "" {
		return ErrEvalRunNotFound
	}
	return fmt.Errorf("%w: %s", ErrEvalRunNotFound, reason)
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
