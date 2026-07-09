package workflow

import "errors"

var (
	// ErrUnknownWorkflow is returned when no workflow matches a name.
	ErrUnknownWorkflow = errors.New("unknown workflow")
	// ErrInvalid marks a workflow that fails validation.
	ErrInvalid = errors.New("invalid workflow")
	// ErrUnknownVerb marks a step whose verb is not recognized.
	ErrUnknownVerb = errors.New("unknown verb")
)
