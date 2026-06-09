package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidFrame             = errors.New("invalid frame")
	ErrAuthExpired              = errors.New("auth expired")
	ErrPermissionDenied         = errors.New("permission denied")
	ErrDeliveryUnavailable      = errors.New("delivery unavailable")
	ErrAckOutOfVisibleRange     = errors.New("ack out of visible range")
	ErrNoOnlineSession          = errors.New("no online session")
	ErrSessionQueueFull         = errors.New("session queue full")
	ErrSessionEvicted           = errors.New("session evicted")
	ErrUnsupportedDeliveryEvent = errors.New("unsupported delivery event")
)

func NewInvalidFrame(reason string) error {
	if reason == "" {
		return ErrInvalidFrame
	}
	return fmt.Errorf("%w: %s", ErrInvalidFrame, reason)
}

func NewAuthExpired(reason string) error {
	if reason == "" {
		return ErrAuthExpired
	}
	return fmt.Errorf("%w: %s", ErrAuthExpired, reason)
}

func NewDeliveryUnavailable(reason string) error {
	if reason == "" {
		return ErrDeliveryUnavailable
	}
	return fmt.Errorf("%w: %s", ErrDeliveryUnavailable, reason)
}

func NewAckOutOfVisibleRange(reason string) error {
	if reason == "" {
		return ErrAckOutOfVisibleRange
	}
	return fmt.Errorf("%w: %s", ErrAckOutOfVisibleRange, reason)
}
