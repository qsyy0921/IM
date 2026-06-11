package types

import "errors"

var (
	ErrInvalidArgument        = errors.New("invalid argument")
	ErrContactRequestNotFound = errors.New("contact request not found")
	ErrContactNotFound        = errors.New("contact not found")
	ErrContactAlreadyExists   = errors.New("contact already exists")
	ErrContactRequestConflict = errors.New("contact request conflict")
	ErrPermissionDenied       = errors.New("permission denied")
	ErrDBWriteFailed          = errors.New("db write failed")
	ErrDBReadFailed           = errors.New("db read failed")
	ErrOutboxWriteFailed      = errors.New("outbox write failed")
	ErrServiceOverloaded      = errors.New("service overloaded")
)

func NewInvalidArgument(message string) error {
	return errors.Join(ErrInvalidArgument, errors.New(message))
}

func NewContactRequestNotFound(message string) error {
	return errors.Join(ErrContactRequestNotFound, errors.New(message))
}

func NewContactNotFound(message string) error {
	return errors.Join(ErrContactNotFound, errors.New(message))
}

func NewContactAlreadyExists(message string) error {
	return errors.Join(ErrContactAlreadyExists, errors.New(message))
}

func NewContactRequestConflict(message string) error {
	return errors.Join(ErrContactRequestConflict, errors.New(message))
}

func NewPermissionDenied(message string) error {
	return errors.Join(ErrPermissionDenied, errors.New(message))
}

func NewDBWriteFailed(message string) error {
	return errors.Join(ErrDBWriteFailed, errors.New(message))
}

func NewDBReadFailed(message string) error {
	return errors.Join(ErrDBReadFailed, errors.New(message))
}

func NewOutboxWriteFailed(message string) error {
	return errors.Join(ErrOutboxWriteFailed, errors.New(message))
}

func NewServiceOverloaded(message string) error {
	return errors.Join(ErrServiceOverloaded, errors.New(message))
}
