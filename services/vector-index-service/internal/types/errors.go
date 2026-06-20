package types

import "errors"

var (
	ErrInvalidArgument    = errors.New("invalid argument")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrNotFound           = errors.New("not found")
	ErrAlreadyExists      = errors.New("already exists")
	ErrFailedPrecondition = errors.New("failed precondition")
	ErrUnavailable        = errors.New("unavailable")
	ErrDBReadFailed       = errors.New("db read failed")
	ErrDBWriteFailed      = errors.New("db write failed")
)

type typedError struct {
	cause   error
	message string
}

func (err typedError) Error() string {
	if err.message == "" {
		return err.cause.Error()
	}
	return err.message
}

func (err typedError) Unwrap() error {
	return err.cause
}

func NewInvalidArgument(message string) error {
	return typedError{cause: ErrInvalidArgument, message: message}
}

func NewPermissionDenied(message string) error {
	return typedError{cause: ErrPermissionDenied, message: message}
}

func NewNotFound(message string) error {
	return typedError{cause: ErrNotFound, message: message}
}

func NewAlreadyExists(message string) error {
	return typedError{cause: ErrAlreadyExists, message: message}
}

func NewFailedPrecondition(message string) error {
	return typedError{cause: ErrFailedPrecondition, message: message}
}

func NewUnavailable(message string) error {
	return typedError{cause: ErrUnavailable, message: message}
}

func NewDBReadFailed(message string) error {
	return typedError{cause: ErrDBReadFailed, message: message}
}

func NewDBWriteFailed(message string) error {
	return typedError{cause: ErrDBWriteFailed, message: message}
}
