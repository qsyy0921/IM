package postgres

import "errors"

var ErrNotImplemented = errors.New("postgres repository is not implemented")
var ErrRepositoryNotConfigured = errors.New("postgres repository pool is not configured")
