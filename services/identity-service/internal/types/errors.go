package types

import "errors"

var (
	ErrInvalidArgument           = errors.New("invalid argument")
	ErrPermissionDenied          = errors.New("permission denied")
	ErrDeviceRevoked             = errors.New("device revoked")
	ErrSessionRevoked            = errors.New("session revoked")
	ErrDeviceNotFound            = errors.New("device not found")
	ErrSessionNotFound           = errors.New("session not found")
	ErrDBReadFailed              = errors.New("db read failed")
	ErrDBWriteFailed             = errors.New("db write failed")
	ErrTokenSigningFailed        = errors.New("token signing failed")
	ErrUserAlreadyExists         = errors.New("user already exists")
	ErrInvalidCredentials        = errors.New("invalid credentials")
	ErrInvalidRefreshToken       = errors.New("invalid refresh token")
	ErrRefreshTokenReuseDetected = errors.New("refresh token reuse detected")
)

type serviceError struct {
	cause   error
	message string
}

func (err serviceError) Error() string {
	if err.message == "" {
		return err.cause.Error()
	}
	return err.cause.Error() + ": " + err.message
}

func (err serviceError) Unwrap() error { return err.cause }

func wrap(cause error, message string) error {
	if message == "" {
		return cause
	}
	return serviceError{cause: cause, message: message}
}

func NewInvalidArgument(message string) error     { return wrap(ErrInvalidArgument, message) }
func NewPermissionDenied(message string) error    { return wrap(ErrPermissionDenied, message) }
func NewDeviceRevoked(message string) error       { return wrap(ErrDeviceRevoked, message) }
func NewSessionRevoked(message string) error      { return wrap(ErrSessionRevoked, message) }
func NewDeviceNotFound(message string) error      { return wrap(ErrDeviceNotFound, message) }
func NewSessionNotFound(message string) error     { return wrap(ErrSessionNotFound, message) }
func NewDBReadFailed(message string) error        { return wrap(ErrDBReadFailed, message) }
func NewDBWriteFailed(message string) error       { return wrap(ErrDBWriteFailed, message) }
func NewTokenSigningFailed(message string) error  { return wrap(ErrTokenSigningFailed, message) }
func NewUserAlreadyExists(message string) error   { return wrap(ErrUserAlreadyExists, message) }
func NewInvalidCredentials(message string) error  { return wrap(ErrInvalidCredentials, message) }
func NewInvalidRefreshToken(message string) error { return wrap(ErrInvalidRefreshToken, message) }
func NewRefreshTokenReuseDetected(message string) error {
	return wrap(ErrRefreshTokenReuseDetected, message)
}
