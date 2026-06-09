package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidFrame             = errors.New("invalid frame")
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
