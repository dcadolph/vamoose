package graph

import "errors"

// ErrGraph means a Graph request returned a non-success status.
var ErrGraph = errors.New("graph request failed")
