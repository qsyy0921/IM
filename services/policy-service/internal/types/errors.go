package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument       = errors.New("invalid argument")
	ErrDependencyUnavailable = errors.New("dependency unavailable")
	ErrDBWriteFailed         = errors.New("db write failed")
)

func NewInvalidArgument(reason string) error {
	if reason == "" {
		return ErrInvalidArgument
	}
	return fmt.Errorf("%w: %s", ErrInvalidArgument, reason)
}

func NewDependencyUnavailable(reason string) error {
	if reason == "" {
		return ErrDependencyUnavailable
	}
	return fmt.Errorf("%w: %s", ErrDependencyUnavailable, reason)
}

func NewDBWriteFailed(reason string) error {
	if reason == "" {
		return ErrDBWriteFailed
	}
	return fmt.Errorf("%w: %s", ErrDBWriteFailed, reason)
}
