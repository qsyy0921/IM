package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidArgument       = errors.New("invalid argument")
	ErrPermissionDenied      = errors.New("permission denied")
	ErrFailedPrecondition    = errors.New("failed precondition")
	ErrMediaAssetNotFound    = errors.New("media asset not found")
	ErrUploadSessionNotFound = errors.New("upload session not found")
	ErrAlreadyExists         = errors.New("already exists")
	ErrProviderUnavailable   = errors.New("provider unavailable")
	ErrDBReadFailed          = errors.New("db read failed")
	ErrDBWriteFailed         = errors.New("db write failed")
)

func NewInvalidArgument(reason string) error {
	return wrap(ErrInvalidArgument, reason)
}

func NewPermissionDenied(reason string) error {
	return wrap(ErrPermissionDenied, reason)
}

func NewFailedPrecondition(reason string) error {
	return wrap(ErrFailedPrecondition, reason)
}

func NewMediaAssetNotFound(reason string) error {
	return wrap(ErrMediaAssetNotFound, reason)
}

func NewUploadSessionNotFound(reason string) error {
	return wrap(ErrUploadSessionNotFound, reason)
}

func NewAlreadyExists(reason string) error {
	return wrap(ErrAlreadyExists, reason)
}

func NewProviderUnavailable(reason string) error {
	return wrap(ErrProviderUnavailable, reason)
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
