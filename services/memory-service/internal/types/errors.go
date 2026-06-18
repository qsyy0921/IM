package types

import "errors"

var (
	ErrInvalidArgument    = errors.New("invalid memory request")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrMemoryUnavailable  = errors.New("memory unavailable")
	ErrProjectionStale    = errors.New("memory projection stale")
	ErrMemoryNotFound     = errors.New("memory not found")
	ErrDBReadFailed       = errors.New("memory db read failed")
	ErrDBWriteFailed      = errors.New("memory db write failed")
	ErrUnsupportedPayload = errors.New("unsupported memory payload")
)

func NewInvalidArgument(_ string) error {
	return ErrInvalidArgument
}

func NewDBReadFailed(_ string) error {
	return ErrDBReadFailed
}

func NewDBWriteFailed(_ string) error {
	return ErrDBWriteFailed
}

func NewUnsupportedPayload(_ string) error {
	return ErrUnsupportedPayload
}
