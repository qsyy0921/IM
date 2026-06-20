package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument    = errors.New("invalid argument")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrNotFound           = errors.New("not found")
	ErrFailedPrecondition = errors.New("failed precondition")
	ErrUnavailable        = errors.New("unavailable")
	ErrDBReadFailed       = errors.New("db read failed")
	ErrDBWriteFailed      = errors.New("db write failed")
)

func NewInvalidArgument(reason string) error {
	return wrap(ErrInvalidArgument, reason)
}

func NewPermissionDenied(reason string) error {
	return wrap(ErrPermissionDenied, reason)
}

func NewNotFound(reason string) error {
	return wrap(ErrNotFound, reason)
}

func NewFailedPrecondition(reason string) error {
	return wrap(ErrFailedPrecondition, reason)
}

func NewUnavailable(reason string) error {
	return wrap(ErrUnavailable, reason)
}

func NewDBReadFailed(reason string) error {
	return wrap(ErrDBReadFailed, reason)
}

func NewDBWriteFailed(reason string) error {
	return wrap(ErrDBWriteFailed, reason)
}

func wrap(base error, reason string) error {
	if reason == "" {
		return base
	}
	return fmt.Errorf("%w: %s", base, reason)
}
