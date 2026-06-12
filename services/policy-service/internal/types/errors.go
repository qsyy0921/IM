package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument       = errors.New("invalid argument")
	ErrDependencyUnavailable = errors.New("dependency unavailable")
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
