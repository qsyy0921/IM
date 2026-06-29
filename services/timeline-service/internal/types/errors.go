package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrDBReadFailed        = errors.New("db read failed")
	ErrDBWriteFailed       = errors.New("db write failed")
	ErrIdempotencyConflict = errors.New("idempotency conflict")
)

func NewInvalidArgument(reason string) error {
	if reason == "" {
		return ErrInvalidArgument
	}
	return fmt.Errorf("%w: %s", ErrInvalidArgument, reason)
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

func NewIdempotencyConflict(reason string) error {
	if reason == "" {
		return ErrIdempotencyConflict
	}
	return fmt.Errorf("%w: %s", ErrIdempotencyConflict, reason)
}
