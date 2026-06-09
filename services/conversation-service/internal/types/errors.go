package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument      = errors.New("invalid argument")
	ErrConversationNotFound = errors.New("conversation not found")
	ErrMemberNotActive      = errors.New("conversation member is not active")
	ErrDBReadFailed         = errors.New("db read failed")
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

func NewMemberNotActive(reason string) error {
	if reason == "" {
		return ErrMemberNotActive
	}
	return fmt.Errorf("%w: %s", ErrMemberNotActive, reason)
}

func NewDBReadFailed(reason string) error {
	if reason == "" {
		return ErrDBReadFailed
	}
	return fmt.Errorf("%w: %s", ErrDBReadFailed, reason)
}
