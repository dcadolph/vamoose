package calendar

import "errors"

var (
	// ErrNotFound means the requested hold does not exist.
	ErrNotFound = errors.New("hold not found")
	// ErrNoManager means the signed-in user has no manager set in the directory.
	ErrNoManager = errors.New("no manager in directory")
)
