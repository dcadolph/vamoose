package calendar

import "errors"

var (
	// ErrNotFound means the requested hold does not exist.
	ErrNotFound = errors.New("hold not found")
	// ErrNoManager means the signed-in user has no manager set in the directory.
	ErrNoManager = errors.New("no manager in directory")
	// ErrUnknownProvider means no calendar provider is registered under the name.
	ErrUnknownProvider = errors.New("unknown calendar provider")
	// ErrNoDirectory means the provider has no directory to resolve peers from,
	// so the team must be set explicitly.
	ErrNoDirectory = errors.New("no directory for this provider")
)
