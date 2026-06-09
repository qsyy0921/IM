package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument        = errors.New("invalid argument")
	ErrPermissionDenied       = errors.New("permission denied")
	ErrReadOutOfVisibleRange  = errors.New("read out of visible range")
	ErrReadOutOfReceivedRange = errors.New("read out of received range")
	ErrReceiptNotFound        = errors.New("receipt not found")
	ErrProjectionLagging      = errors.New("projection lagging")
	ErrDBReadFailed           = errors.New("db read failed")
	ErrDBWriteFailed          = errors.New("db write failed")
	ErrServiceOverloaded      = errors.New("service overloaded")
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

func NewReadOutOfVisibleRange(reason string) error {
	if reason == "" {
		return ErrReadOutOfVisibleRange
	}
	return fmt.Errorf("%w: %s", ErrReadOutOfVisibleRange, reason)
}

func NewReadOutOfReceivedRange(reason string) error {
	if reason == "" {
		return ErrReadOutOfReceivedRange
	}
	return fmt.Errorf("%w: %s", ErrReadOutOfReceivedRange, reason)
}

func NewReceiptNotFound(reason string) error {
	if reason == "" {
		return ErrReceiptNotFound
	}
	return fmt.Errorf("%w: %s", ErrReceiptNotFound, reason)
}

func NewProjectionLagging(reason string) error {
	if reason == "" {
		return ErrProjectionLagging
	}
	return fmt.Errorf("%w: %s", ErrProjectionLagging, reason)
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
