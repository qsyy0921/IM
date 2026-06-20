package types

import "errors"

var (
	ErrInvalidArgument    = errors.New("invalid argument")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrAlreadyExists      = errors.New("already exists")
	ErrNotFound           = errors.New("not found")
	ErrFailedPrecondition = errors.New("failed precondition")
	ErrUnavailable        = errors.New("unavailable")
	ErrDBReadFailed       = errors.New("db read failed")
	ErrDBWriteFailed      = errors.New("db write failed")
)

type MessageError struct {
	Code    error
	Message string
}

func (err MessageError) Error() string {
	if err.Message == "" {
		return err.Code.Error()
	}
	return err.Message
}

func (err MessageError) Unwrap() error {
	return err.Code
}

func NewInvalidArgument(message string) error {
	return MessageError{Code: ErrInvalidArgument, Message: message}
}

func NewPermissionDenied(message string) error {
	return MessageError{Code: ErrPermissionDenied, Message: message}
}

func NewAlreadyExists(message string) error {
	return MessageError{Code: ErrAlreadyExists, Message: message}
}

func NewNotFound(message string) error {
	return MessageError{Code: ErrNotFound, Message: message}
}

func NewFailedPrecondition(message string) error {
	return MessageError{Code: ErrFailedPrecondition, Message: message}
}

func NewUnavailable(message string) error {
	return MessageError{Code: ErrUnavailable, Message: message}
}

func NewDBReadFailed(message string) error {
	return MessageError{Code: ErrDBReadFailed, Message: message}
}

func NewDBWriteFailed(message string) error {
	return MessageError{Code: ErrDBWriteFailed, Message: message}
}
