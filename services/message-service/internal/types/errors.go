package types

import "errors"

var (
	ErrPermissionDenied     = errors.New("permission denied")
	ErrSequencerUnavailable = errors.New("sequencer unavailable")
)

func NewPermissionDenied(reason string) error {
	if reason == "" {
		return ErrPermissionDenied
	}
	return errors.New("permission denied: " + reason)
}

func NewSequencerUnavailable(reason string) error {
	if reason == "" {
		return ErrSequencerUnavailable
	}
	return errors.New("sequencer unavailable: " + reason)
}
