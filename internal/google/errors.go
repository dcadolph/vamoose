package google

import "errors"

// ErrGoogle means a Calendar API request returned a non-success status.
var ErrGoogle = errors.New("google calendar request failed")
